package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"gopkg.in/yaml.v2"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

/* Config */

type CommandConfig struct{}

type CacheConfig struct {
	MaxEntries int `yaml:"max_entries"`
}

type Config struct {
	// List of commands to memoize
	Commands map[string]CommandConfig `yaml:"memoize_commands"`
	Cache    CacheConfig              `yaml:"cache"`
}

/* Storage */
type Store struct {
	Dir string
}

type CacheKey struct {
	Hash string
}

func (c *Store) KeyFrom(command string, args []string) CacheKey {
	concatCmd := command + " " + strings.Join(args, " ")
	h := sha256.Sum256([]byte(concatCmd))
	return CacheKey{
		Hash: fmt.Sprintf("%x", h[:]),
	}
}

func (s *Store) stdoutPath(key CacheKey) string {
	return filepath.Join(s.KeyDir(key), "out")
}

func (s *Store) stderrPath(key CacheKey) string {
	return filepath.Join(s.KeyDir(key), "err")
}

func (s *Store) exitcodePath(key CacheKey) string {
	return filepath.Join(s.KeyDir(key), "status")
}

func (s *Store) WriteToCache(key CacheKey, stdout, stderr []byte, exitCode int) error {
	cacheDir := s.KeyDir(key)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return err
	}

	if err := os.WriteFile(s.stdoutPath(key), stdout, 0644); err != nil {
		return err
	}
	if err := os.WriteFile(s.stderrPath(key), stderr, 0644); err != nil {
		return err
	}
	if err := os.WriteFile(s.exitcodePath(key), []byte(fmt.Sprint(exitCode)), 0644); err != nil {
		return err
	}

	return nil
}

func (s *Store) ReadFromCache(key CacheKey) (stdout, stderr []byte, exitCode int, err error) {
	stdout, err = os.ReadFile(s.stdoutPath(key))
	if err != nil {
		return
	}
	stderr, err = os.ReadFile(s.stderrPath(key))
	if err != nil {
		return
	}
	exitCodeBytes, err := os.ReadFile(s.exitcodePath(key))
	if err != nil {
		return
	}
	exitCode, err = strconv.Atoi(string(exitCodeBytes))
	return
}

/* Cash */

type Cash struct {
	ConfigPath string
	Dir        string
	Config     Config
	Store      *Store
}

func NewCash(configPath, dir string) *Cash {
	return &Cash{
		ConfigPath: configPath,
		Dir:        dir,
		Store: &Store{
			Dir: filepath.Join(dir, "data"),
		},
	}
}

func (m *Cash) LoadConfig() error {
	configFile, err := os.Open(m.ConfigPath)
	if err != nil {
		return fmt.Errorf("failed to open config file: %w", err)
	}
	defer configFile.Close()

	decoder := yaml.NewDecoder(configFile)
	if err := decoder.Decode(&m.Config); err != nil {
		return fmt.Errorf("failed to decode config: %w", err)
	}

	return nil
}

func (m *Cash) IsCommandMemoized(command string) bool {
	_, ok := m.Config.Commands[command]
	return ok
}

func (m *Cash) Init() error {
	if err := m.InitializeEnv(); err != nil {
		return err
	}

	if err := m.LoadConfig(); err != nil {
		return err
	}

	return m.CreateActivateScript()
}

func (m *Cash) BinDir() string {
	return filepath.Join(m.Dir, "bin")
}

func (m *Cash) OgBinDir() string {
	return filepath.Join(m.Dir, "ogbin")
}

// LinkCommands synchronizes actual symlinks with config
func (m *Cash) LinkCommands() error {
	// Ensure DIR/bin and DIR/ogbin exist
	binDir := m.BinDir()
	ogbinDir := m.OgBinDir()
	for _, dir := range []string{binDir, ogbinDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create %s, directory: %w", dir, err)
		}
	}

	// Path to the cash binary
	cashPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get cash executable path: %w", err)
	}

	// Iterate over commands from the config file
	for cmd := range m.Config.Commands {
		symlinkPath := filepath.Join(binDir, cmd)
		ogbinPath := filepath.Join(ogbinDir, cmd)
		// Remove existing symlinks if they exists. Creating all new symlinks
		// means targets are always up to date after running `cash link`.
		if _, err := os.Lstat(symlinkPath); err == nil {
			if err := os.Remove(symlinkPath); err != nil {
				return fmt.Errorf("failed to remove existing symlink for %s: %w", cmd, err)
			}
		}
		if _, err := os.Lstat(ogbinPath); err == nil {
			if err := os.Remove(ogbinPath); err != nil {
				return fmt.Errorf("failed to remove existing symlink for %s: %w", cmd, err)
			}
		}

		// Create symlink ogbin/<cmd> -> original absolute path to <cmd>
		// to avoid recursive cash invocations
		if cmdPath, err := exec.LookPath(cmd); err == nil {
			// create a link to the original command in the ogbin directory
			if err := os.Symlink(cmdPath, ogbinPath); err != nil {
				return fmt.Errorf("failed to create symlink for %s: %w", cmd, err)
			}
		}

		// Create symlink bin/<cmd> -> cash
		if err := os.Symlink(cashPath, symlinkPath); err != nil {
			return fmt.Errorf("failed to create symlink for %s: %w", cmd, err)
		}

		fmt.Printf("Created symlink for %s\n", cmd)
	}

	// Iterate over symlinks to delete any that are not in the config
	entries, err := os.ReadDir(binDir)
	if err != nil {
		return fmt.Errorf("failed to read bin directory: %w", err)
	}
	for _, entry := range entries {
		if entry.Name() == "activate" {
			continue
		}
		if _, ok := m.Config.Commands[entry.Name()]; !ok {
			symlinkPath := filepath.Join(binDir, entry.Name())
			if err := os.Remove(symlinkPath); err != nil {
				return fmt.Errorf("failed to remove symlink for %s: %w", entry.Name(), err)
			}
			fmt.Printf("Removed symlink for %s\n", entry.Name())
		}
	}

	return nil
}

func (m *Cash) CreateActivateScript() error {
	// Define the path to the activate script within the bin directory
	activateScriptPath := filepath.Join(m.BinDir(), "activate")

	// Define the content of the activate script
	activateScriptContent := `#!/bin/bash
# DIR/bin/activate - script to activate cash

# Resolve the directory of this script
CASH_BIN="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Save the current directory to revert back on deactivation
export CASH_OLDPWD="$PWD"

# Check if already activated
if [ -z "$CASH_DIR" ]; then

    # Function to deactivate cash and restore original environment
    deactivate_cash() {
        if [ -z "$CASH_DIR" ]; then
            echo "cash is not activated."
            return
        fi

        # Restore the original PATH
        export PATH="$CASH_OLD_PATH"
        unset CASH_OLD_PATH
        unset CASH_DIR
        unset CASH_CONFIG

        # Remove the deactivate function
        unset -f deactivate_cash

        echo "cash deactivated."
    }

    export CASH_DIR="$(cd "$CASH_BIN/.." && pwd)"
	export CASH_OLD_PATH="$PATH"
	export CASH_CONFIG="$CASH_DIR/cash.yml"
	export PATH="$CASH_DIR/bin:$PATH"

	echo "cash activated. Use 'deactivate_cash' to deactivate."
else
	echo "cash is already activated."
fi
`

	// Ensure the bin directory exists
	if err := os.MkdirAll(m.BinDir(), 0755); err != nil {
		return fmt.Errorf("failed to create bin directory: %w", err)
	}

	// Write the activate script content to the file
	if err := os.WriteFile(activateScriptPath, []byte(activateScriptContent), 0755); err != nil {
		return fmt.Errorf("failed to write activate script: %w", err)
	}

	fmt.Printf("Created activate script at %s\n", activateScriptPath)
	return nil
}

// InitializeEnv creates the cache directory and initializes the config file
func (c *Cash) InitializeEnv() error {
	// Create the directory if it does not exist
	if _, err := os.Stat(c.Dir); os.IsNotExist(err) {
		if err := os.MkdirAll(c.Dir, 0755); err != nil {
			return fmt.Errorf("failed to create cache directory: %w", err)
		}
	}

	// Create the config file if it does not exist
	if _, err := os.Stat(c.ConfigPath); os.IsNotExist(err) {
		// Initialize a default config
		defaultConfig := Config{
			Commands: make(map[string]CommandConfig, 0),
			Cache: CacheConfig{
				MaxEntries: 1000,
			},
		}
		configFile, err := os.Create(c.ConfigPath)
		if err != nil {
			return fmt.Errorf("failed to create config file: %w", err)
		}
		defer configFile.Close()

		encoder := yaml.NewEncoder(configFile)
		if err := encoder.Encode(defaultConfig); err != nil {
			return fmt.Errorf("failed to encode default config: %w", err)
		}
	}

	return nil
}

func (s *Store) KeyDir(key CacheKey) string {
	return filepath.Join(s.Dir, key.Hash)
}

func (s *Store) Exists(key CacheKey) bool {
	_, err := os.Stat(s.KeyDir(key))
	return !os.IsNotExist(err)
}

func (m *Cash) HandleMemoizedCommand(cmd string, args []string) {
	key := m.Store.KeyFrom(cmd, args)

	if m.Store.Exists(key) {
		stdout, stderr, exitCode, err := m.Store.ReadFromCache(key)
		if err != nil {
			fmt.Printf("Failed to read from cache: %v\n", err)
			return
		}
		fmt.Println("Output retrieved from cache:")
		fmt.Println("Stdout:", string(stdout))
		fmt.Println("Stderr:", string(stderr))
		fmt.Println("Exit Code:", exitCode)
	} else {
		fmt.Println("Prepping command", cmd, args)
		ogCmdPath := filepath.Join(m.OgBinDir(), cmd)
		cmdExec := exec.Command(ogCmdPath, args...)
		var stdoutBuf, stderrBuf bytes.Buffer
		cmdExec.Stdout = &stdoutBuf
		cmdExec.Stderr = &stderrBuf
		fmt.Println("Running command")
		err := cmdExec.Run()
		fmt.Println("Done running command", stdoutBuf.Bytes(), stderrBuf.Bytes())
		exitCode := 0
		if err != nil {
			if exitError, ok := err.(*exec.ExitError); ok {
				exitCode = exitError.ExitCode()
			} else {
				fmt.Printf("Error executing command: %v\n", err)
				return
			}
		} else {
			exitCode = cmdExec.ProcessState.ExitCode()
		}
		fmt.Println("Done getting exit code")

		err = m.Store.WriteToCache(key, stdoutBuf.Bytes(), stderrBuf.Bytes(), exitCode)
		if err != nil {
			fmt.Printf("Failed to write to cache: %v\n", err)
			return
		}

		fmt.Println("Command executed and output cached")
	}
}

func main() {
	// This program is used both for controlling cash (e.g. `cash init`) and
	// for intercepting memoized commands. Use $0 to determine which is
	// happening.
	invokedCmd := filepath.Base(os.Args[0])
	switch invokedCmd {
	case "cash":
		if len(os.Args) < 2 {
			fmt.Println("Usage: cash <command> [arguments]")
			return
		}
		handleCashSubcommand(os.Args[1], os.Args[2:])
	default:
		handleMemoizedCommand(invokedCmd, os.Args[1:])
		return
	}
}

func handleCashSubcommand(subcommand string, args []string) {
	switch subcommand {
	case "init":
		handleInit(args)
	case "link":
		handleLink(args)
	case "add":
		handleAdd(args)
	default:
		fmt.Println("Invalid command. Available commands are: init, link")
	}
}

func handleInit(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: cash init <DIR>")
		return
	}
	dir := args[0]

	cash := NewCash(filepath.Join(dir, "cash.yml"), dir)
	if err := cash.Init(); err != nil {
		fmt.Printf("Error initializing cash: %v\n", err)
	}
}

func getActiveCashDir() (string, error) {
	dir, ok := os.LookupEnv("CASH_DIR")
	if !ok {
		return "", fmt.Errorf("cash directory not set; please activate first.")
	}
	return dir, nil
}

func loadCashFromDir(dir string) (*Cash, error) {
	return NewCash(filepath.Join(dir, "cash.yml"), dir), nil
}

// Supports both:
// - `cash link DIR`
// - `cash link` while activated
func handleLink(args []string) {
	var err error
	var dir string

	// Get the desired cash dir from the first arg or CASH_DIR
	if len(args) == 1 {
		dir = args[0]
	} else {
		if dir, err = getActiveCashDir(); err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			return
		}
	}

	cash, err := loadCashFromDir(dir)
	if err := cash.LoadConfig(); err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		return
	}

	if err := cash.LinkCommands(); err != nil {
		fmt.Printf("Error creating symlinks: %v\n", err)
	}
}

func handleAdd(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: cash add <command>")
		return
	}

	dir, err := getActiveCashDir()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		return
	}

	cash, err := loadCashFromDir(dir)
	if err := cash.LoadConfig(); err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		return
	}

	command := args[0]
	if cash.IsCommandMemoized(command) {
		fmt.Printf("Command '%s' is already memoized.\n", command)
		return
	}

	cash.Config.Commands[command] = CommandConfig{}
	configFile, err := os.Create(cash.ConfigPath)
	if err != nil {
		fmt.Printf("Error opening config file: %v\n", err)
		return
	}
	defer configFile.Close()

	encoder := yaml.NewEncoder(configFile)
	if err := encoder.Encode(cash.Config); err != nil {
		fmt.Printf("Error encoding config: %v\n", err)
		return
	}

	fmt.Printf("Command '%s' added to memoized commands.\n", command)
}

// HandleMemoizedCommand handles the execution of a memoized command
func handleMemoizedCommand(cmd string, args []string) {
	dir := os.Getenv("CASH_DIR")
	if dir == "" {
		fmt.Println("Error: cash directory not set. Cannot execute memoized command.")
		return
	}

	cash := NewCash(filepath.Join(dir, "cash.yml"), dir)
	if err := cash.LoadConfig(); err != nil {
		fmt.Printf("Error loading config for memoized command '%s': %v\n", cmd, err)
		return
	}
	cash.HandleMemoizedCommand(cmd, args)
}
