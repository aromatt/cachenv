package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

/* Cachenv */

type Cachenv struct {
	ConfigPath string
	Dir        string
	BinDir     string
	OldBinDir  string
	Config     Config
	Store      *Store
}

func NewCachenv(configPath, dir string) *Cachenv {
	return &Cachenv{
		ConfigPath: configPath,
		Dir:        dir,
		BinDir:     filepath.Join(dir, "bin"),
		OldBinDir:  filepath.Join(dir, "bin.old"),
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

func (m *Cachenv) LinkCommand(cmd string) error {
	symlinkPath := filepath.Join(m.BinDir, cmd)
	oldbinPath := filepath.Join(m.OldBinDir, cmd)

	// Path to the cachenv binary
	cachenvPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get cachenv executable path: %w", err)
	}

	// Remove existing symlink if it exists. Creating all new symlinks
	// means targets are always up to date after running `cachenv link`.
	if _, err := os.Lstat(symlinkPath); err == nil {
		if err := os.Remove(symlinkPath); err != nil {
			return fmt.Errorf("failed to remove existing symlink for %s: %w", cmd, err)
		}
	}
	if _, err := os.Lstat(oldbinPath); err == nil {
		if err := os.Remove(oldbinPath); err != nil {
			return fmt.Errorf("failed to remove existing symlink for %s: %w", cmd, err)
		}
	}

	// Create symlink oldbin/<cmd> -> original absolute path to <cmd>
	// to avoid recursive cachenv invocations
	if cmdPath, err := exec.LookPath(cmd); err == nil {
		// create a link to the original command in the oldbin directory
		if err := os.Symlink(cmdPath, oldbinPath); err != nil {
			return fmt.Errorf("failed to create symlink for %s: %w", cmd, err)
		}
	}

	// Create symlink bin/<cmd> -> cachenv
	if err := os.Symlink(cachenvPath, symlinkPath); err != nil {
		return fmt.Errorf("failed to create symlink for %s: %w", cmd, err)
	}

	fmt.Printf("Created symlink for %s\n", cmd)
	return nil
}

// LinkCommands synchronizes actual symlinks with config
func (m *Cachenv) LinkCommands() error {
	// Ensure DIR/bin and DIR/bin.old exist
	for _, dir := range []string{m.BinDir, m.OldBinDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create %s, directory: %w", dir, err)
		}
	}

	// Iterate over commands from the config file
	for cmd := range m.Config.Commands {
		m.LinkCommand(cmd)
	}

	// Iterate over symlinks to delete any that are not in the config
	entries, err := os.ReadDir(m.BinDir)
	if err != nil {
		return fmt.Errorf("failed to read bin directory: %w", err)
	}
	for _, entry := range entries {
		if entry.Name() == "activate" {
			continue
		}
		if _, ok := m.Config.Commands[entry.Name()]; !ok {
			symlinkPath := filepath.Join(m.BinDir, entry.Name())
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
	activateScriptPath := filepath.Join(m.Dir, "activate")

	cachenvExecPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get cachenv executable path: %w", err)
	}

	// Define the content of the activate script
	activateScriptContent := fmt.Sprintf(`
# This script must be invoked from your shell via 'source <cachenv>/activate'.
# This script is heavily inspired by virtualenv's activate script.

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
    echo "You must source this script: \$ source $0" >&2
    exit 33
fi

# Check if already activated
if ! [ -z "$CACHENV" ]; then
    echo "cachenv is already activated."
    exit 0
fi

# Function to deactivate cachenv and restore original environment
deactivate_cachenv() {
    if [ -z "$CACHENV" ]; then
        echo "cachenv is not activated."
        return
    fi

    # Restore the original PATH
    export PATH="$_CACHENV_OLD_PATH"
    unset _CACHENV_OLD_PATH
    unset _CACHENV_EXECUTABLE
    unset CACHENV

    # Remove shell functions
    unset -f deactivate_cachenv
    unset -f cachenv

    # Needed for some commands after changing PATH
    hash -r 2>/dev/null

    # Restore old prompt
    if ! [ -z "${_CACHENV_OLD_PS1+_}" ] ; then
        PS1="$_CACHENV_OLD_PS1"
        export PS1
        unset _CACHENV_OLD_PS1
    fi
}

# Intercept cachenv itself so that we can run 'hash -r' after adding new symlinks
cachenv() {
    # Another way to run deactivate
    if [ "$1" = "deactivate" ]; then
        deactivate_cachenv
        return
    fi

    "$_CACHENV_EXECUTABLE" "$@"
    local cachenv_exit_code=$?

    if [ "$1" = "add" ] || [ "$1" = "link" ]; then
        # Needed for some commands after changing PATH
		hash -r 2>/dev/null
    fi

	return $cachenv_exit_code
}

export CACHENV="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
export _CACHENV_OLD_PATH="$PATH"
export _CACHENV_EXECUTABLE="%s"

export PATH="$CACHENV/bin:$PATH"

# Needed for some commands after changing PATH
hash -r 2>/dev/null

# Add a prefix to the shell prompt
_CACHENV_OLD_PS1="${PS1-}"
if [ "x" != x ] ; then
    PS1="${PS1-}"
else
    PS1="($(basename "$CACHENV")) ${PS1-}"
fi
export PS1
`, cachenvExecPath)

	// Ensure the bin directory exists
	if err := os.MkdirAll(m.BinDir, 0755); err != nil {
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

func (m *Cachenv) HandleMemoizedCommand(cmd string, args []string) int {
	key := m.Store.KeyFrom(cmd, args)
	var stdout, stderr []byte
	var exitCode int
	var err error

	if m.Store.Exists(key) {
		stdout, stderr, exitCode, err = m.Store.ReadFromCache(key)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read from cache: %v\n", err)
			return -1
		}
	} else {
		realCmdPath := filepath.Join(m.OldBinDir, cmd)
		cmd := exec.Command(realCmdPath, args...)
		var stdoutBuf, stderrBuf bytes.Buffer
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf

		err = cmd.Run()
		exitCode := 0
		if err != nil {
			if exitError, ok := err.(*exec.ExitError); ok {
				exitCode = exitError.ExitCode()
			} else {
				fmt.Fprintf(os.Stderr, "Error executing command: %v\n", err)
				return -1
			}
		} else {
			exitCode = cmd.ProcessState.ExitCode()
		}

		stdout = stdoutBuf.Bytes()
		stderr = stderrBuf.Bytes()
		err = m.Store.WriteToCache(key, stdout, stderr, exitCode)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write to cache: %v\n", err)
			return -1
		}
	}

	fmt.Fprint(os.Stdout, string(stdout))
	fmt.Fprint(os.Stderr, string(stderr))
	return exitCode
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
		exitCode := handleMemoizedCommand(invokedCmd, os.Args[1:])
		os.Exit(exitCode)
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
	dir, ok := os.LookupEnv("CACHENV")
	if !ok {
		return "", fmt.Errorf("cachenv directory not set; please activate first.")
	}
	return dir, nil
}

func loadCachenvFromDir(dir string) *Cachenv {
	return NewCachenv(filepath.Join(dir, CONFIG_NAME), dir)
}

// - `cachenv link` while activated
func handleLink(args []string) {
	var err error
	var dir string

	if dir, err = getActiveCachenvDir(); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	cachenv := loadCachenvFromDir(dir)
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

	cachenv := loadCachenvFromDir(dir)
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

	if err := cachenv.LinkCommands(); err != nil {
		fmt.Printf("Error creating symlinks: %v\n", err)
	}
}

// HandleMemoizedCommand handles the execution of a memoized command
func handleMemoizedCommand(cmd string, args []string) int {
	dir := os.Getenv("CACHENV")
	if dir == "" {
		fmt.Println("Error: cachenv directory not set. Cannot execute memoized command.")
		return -1
	}

	cachenv := loadCachenvFromDir(dir)
	if err := cachenv.LoadConfig(); err != nil {
		fmt.Printf("Error loading config for memoized command '%s': %v\n", cmd, err)
		return -1
	}
	return cachenv.HandleMemoizedCommand(cmd, args)
}
