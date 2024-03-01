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

const CONFIG_NAME = "config.yml"

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

/* Cachenv */

type Cachenv struct {
	ConfigPath string
	Dir        string
	Config     Config
	Store      *Store
}

func NewCachenv(configPath, dir string) *Cachenv {
	return &Cachenv{
		ConfigPath: configPath,
		Dir:        dir,
		Store: &Store{
			Dir: filepath.Join(dir, "data"),
		},
	}
}

func (m *Cachenv) LoadConfig() error {
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

func (m *Cachenv) IsCommandMemoized(command string) bool {
	_, ok := m.Config.Commands[command]
	return ok
}

func (m *Cachenv) Init() error {
	if err := m.InitializeEnv(); err != nil {
		return err
	}

	if err := m.LoadConfig(); err != nil {
		return err
	}

	return m.CreateActivateScript()
}

func (m *Cachenv) BinDir() string {
	return filepath.Join(m.Dir, "bin")
}

func (m *Cachenv) OgBinDir() string {
	return filepath.Join(m.Dir, "ogbin")
}

// LinkCommands synchronizes actual symlinks with config
func (m *Cachenv) LinkCommands() error {
	// Ensure DIR/bin and DIR/ogbin exist
	binDir := m.BinDir()
	ogbinDir := m.OgBinDir()
	for _, dir := range []string{binDir, ogbinDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create %s, directory: %w", dir, err)
		}
	}

	// Path to the cachenv binary
	cachenvPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get cachenv executable path: %w", err)
	}

	// Iterate over commands from the config file
	for cmd := range m.Config.Commands {
		symlinkPath := filepath.Join(binDir, cmd)
		ogbinPath := filepath.Join(ogbinDir, cmd)
		// Remove existing symlinks if they exists. Creating all new symlinks
		// means targets are always up to date after running `cachenv link`.
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
		// to avoid recursive cachenv invocations
		if cmdPath, err := exec.LookPath(cmd); err == nil {
			// create a link to the original command in the ogbin directory
			if err := os.Symlink(cmdPath, ogbinPath); err != nil {
				return fmt.Errorf("failed to create symlink for %s: %w", cmd, err)
			}
		}

		// Create symlink bin/<cmd> -> cachenv
		if err := os.Symlink(cachenvPath, symlinkPath); err != nil {
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

func (m *Cachenv) CreateActivateScript() error {
	// Define the path to the activate script within the bin directory
	activateScriptPath := filepath.Join(m.BinDir(), "activate")

	// Define the content of the activate script
	activateScriptContent := fmt.Sprintf(`#!/bin/bash
# DIR/bin/activate - script to activate cachenv

# Resolve the directory of this script
CACHENV_BIN="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Save the current directory to revert back on deactivation
export CACHENV_OLDPWD="$PWD"

# Check if already activated
if [ -z "$CACHENV_DIR" ]; then

    # Function to deactivate cachenv and restore original environment
    deactivate_cachenv() {
        if [ -z "$CACHENV_DIR" ]; then
            echo "cachenv is not activated."
            return
        fi

        # Restore the original PATH
        export PATH="$CACHENV_OLD_PATH"
        unset CACHENV_OLD_PATH
        unset CACHENV_DIR
        unset CACHENV_CONFIG

        # Remove the deactivate function
        unset -f deactivate_cachenv

        echo "cachenv deactivated."
    }

    export CACHENV_DIR="$(cd "$CACHENV_BIN/.." && pwd)"
	export CACHENV_OLD_PATH="$PATH"
	export CACHENV_CONFIG="$CACHENV_DIR/%s"
	export PATH="$CACHENV_DIR/bin:$PATH"

	echo "cachenv activated. Use 'deactivate_cachenv' to deactivate."
else
	echo "cachenv is already activated."
fi
`, CONFIG_NAME)

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
func (c *Cachenv) InitializeEnv() error {
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

func (m *Cachenv) HandleMemoizedCommand(cmd string, args []string) {
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
	// This program is used both for controlling cachenv (e.g. `cachenv init`) and
	// for intercepting memoized commands. Use $0 to determine which is
	// happening.
	invokedCmd := filepath.Base(os.Args[0])
	switch invokedCmd {
	case "cachenv":
		if len(os.Args) < 2 {
			fmt.Println("Usage: cachenv <command> [arguments]")
			return
		}
		handleCachenvSubcommand(os.Args[1], os.Args[2:])
	default:
		handleMemoizedCommand(invokedCmd, os.Args[1:])
		return
	}
}

func handleCachenvSubcommand(subcommand string, args []string) {
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
		fmt.Println("Usage: cachenv init <DIR>")
		return
	}
	dir := args[0]

	cachenv := loadCachenvFromDir(dir)
	if err := cachenv.Init(); err != nil {
		fmt.Printf("Error initializing cachenv: %v\n", err)
	}
}

func getActiveCachenvDir() (string, error) {
	dir, ok := os.LookupEnv("CACHENV_DIR")
	if !ok {
		return "", fmt.Errorf("cachenv directory not set; please activate first.")
	}
	return dir, nil
}

func loadCachenvFromDir(dir string) *Cachenv {
	return NewCachenv(filepath.Join(dir, CONFIG_NAME), dir)
}

// Supports both:
// - `cachenv link DIR`
// - `cachenv link` while activated
func handleLink(args []string) {
	var err error
	var dir string

	// Get the desired cachenv dir from the first arg or CACHENV_DIR
	if len(args) == 1 {
		dir = args[0]
	} else {
		if dir, err = getActiveCachenvDir(); err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			return
		}
	}

	cachenv, err := loadCachenvFromDir(dir)
	if err := cachenv.LoadConfig(); err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		return
	}

	if err := cachenv.LinkCommands(); err != nil {
		fmt.Printf("Error creating symlinks: %v\n", err)
	}
}

func handleAdd(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: cachenv add <command>")
		return
	}

	dir, err := getActiveCachenvDir()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		return
	}

	cachenv, err := loadCachenvFromDir(dir)
	if err := cachenv.LoadConfig(); err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		return
	}

	command := args[0]
	if cachenv.IsCommandMemoized(command) {
		fmt.Printf("Command '%s' is already memoized.\n", command)
		return
	}

	cachenv.Config.Commands[command] = CommandConfig{}
	configFile, err := os.Create(cachenv.ConfigPath)
	if err != nil {
		fmt.Printf("Error opening config file: %v\n", err)
		return
	}
	defer configFile.Close()

	encoder := yaml.NewEncoder(configFile)
	if err := encoder.Encode(cachenv.Config); err != nil {
		fmt.Printf("Error encoding config: %v\n", err)
		return
	}

	fmt.Printf("Command '%s' added to memoized commands.\n", command)
}

// HandleMemoizedCommand handles the execution of a memoized command
func handleMemoizedCommand(cmd string, args []string) {
	dir := os.Getenv("CACHENV_DIR")
	if dir == "" {
		fmt.Println("Error: cachenv directory not set. Cannot execute memoized command.")
		return
	}

	cachenv := loadCachenvFromDir(dir)
	if err := cachenv.LoadConfig(); err != nil {
		fmt.Printf("Error loading config for memoized command '%s': %v\n", cmd, err)
		return
	}
	cachenv.HandleMemoizedCommand(cmd, args)
}
