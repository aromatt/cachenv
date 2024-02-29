package main

/* This is a program that memoizes shell commands. */

import (
	"fmt"
	"gopkg.in/yaml.v2"
	"os"
	"os/exec"
	"path/filepath"
)

// Config holds the commands to memoize
type Config struct {
	MemoizeCommands []string `yaml:"memoize_commands"`
}

// Cash represents the memoization environment
type Cash struct {
	ConfigPath string
	CacheDir   string
	Config     Config
}

// NewCash creates a new Cash instance
func NewCash(configPath, cacheDir string) *Cash {
	return &Cash{
		ConfigPath: configPath,
		CacheDir:   cacheDir,
	}
}

// LoadConfig loads the configuration from the YAML file
func (m *Cash) LoadConfig() error {
	yamlFile, err := os.ReadFile(m.ConfigPath)
	if err != nil {
		return err
	}
	err = yaml.Unmarshal(yamlFile, &m.Config)
	if err != nil {
		return err
	}
	return nil
}

// IsCommandMemoized checks if a command is in the memoize list
func (m *Cash) IsCommandMemoized(command string) bool {
	for _, cmd := range m.Config.MemoizeCommands {
		if cmd == command {
			return true
		}
	}
	return false
}

// Init combines the functionality needed for the 'init' subcommand.
func (m *Cash) Init() error {
	if err := initializeEnv(m.CacheDir, m.ConfigPath); err != nil {
		return err
	}

	if err := m.LoadConfig(); err != nil {
		return err
	}

	return m.CreateActivateScript()
}

func (m *Cash) LinkCommands() error {
	// Ensure DIR/bin exists
	binDir := filepath.Join(m.CacheDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("failed to create bin directory: %w", err)
	}

	// Path to the cash binary
	cashPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get cash executable path: %w", err)
	}

	// Iterate over commands from the config file
	for _, cmd := range m.Config.MemoizeCommands {
		symlinkPath := filepath.Join(binDir, cmd)
		// Remove existing symlink if it exists to avoid error on creating new one
		if _, err := os.Lstat(symlinkPath); err == nil {
			if err := os.Remove(symlinkPath); err != nil {
				return fmt.Errorf("failed to remove existing symlink for %s: %w", cmd, err)
			}
		}

		// Create a new symlink
		if err := os.Symlink(cashPath, symlinkPath); err != nil {
			return fmt.Errorf("failed to create symlink for %s: %w", cmd, err)
		}
		fmt.Printf("Created symlink for %s\n", cmd)
	}

	return nil
}

func (m *Cash) CreateActivateScript() error {
	// Define the path to the activate script within the bin directory
	activateScriptPath := filepath.Join(m.CacheDir, "bin", "activate")

	// Define the content of the activate script
	activateScriptContent := `#!/bin/bash
# DIR/bin/activate - script to activate cash

# Save the current directory to revert back on deactivation
export CASH_OLDPWD="$PWD"

# Function to deactivate cash and restore original environment
deactivate_cash() {
    # Restore the original PATH
    export PATH="$CASH_OLD_PATH"
    unset CASH_OLD_PATH
    unset CASH_DIR
    unset CASH_CONFIG
    echo "cash deactivated."
}

# Store the original PATH to restore it upon deactivation
export CASH_OLD_PATH="$PATH"

# Directory where cash is located
export CASH_DIR="$(cd "$(dirname $0/..)" && pwd)"

# Set the path to the cash.yml configuration file
export CASH_CONFIG="$CASH_DIR/cash.yml"

# Prepend the cash bin directory to PATH so any custom scripts or overrides take precedence
export PATH="$CASH_DIR/bin:$PATH"

echo "cash activated. Use 'deactivate_cash' to deactivate."
`

	// Ensure the bin directory exists
	binDir := filepath.Dir(activateScriptPath)
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("failed to create bin directory: %w", err)
	}

	// Write the activate script content to the file
	if err := os.WriteFile(activateScriptPath, []byte(activateScriptContent), 0755); err != nil {
		return fmt.Errorf("failed to write activate script: %w", err)
	}

	fmt.Printf("Created activate script at %s\n", activateScriptPath)
	return nil
}
func main() {

	invokedCmd := filepath.Base(os.Args[0])

	// Check if the program is invoked via symlink (a memoized command)
	if invokedCmd != "cash" {
		handleMemoizedCommand(invokedCmd, os.Args[1:])
		return
	}

	if len(os.Args) < 2 {
		fmt.Println("Usage: cash <command> [arguments]")
		return
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "init":
		handleInit(args)
	case "link":
		handleLink(args)
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

func handleLink(args []string) {
	var dir string
	if len(args) == 1 {
		dir = args[0]
	} else {
		dir = os.Getenv("CASH_DIR")
		if dir == "" {
			fmt.Println("Error: cash directory not set. Please provide DIR or activate cash.")
			return
		}
	}

	cash := NewCash(filepath.Join(dir, "cash.yml"), dir)
	if err := cash.LoadConfig(); err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		return
	}

	if err := cash.LinkCommands(); err != nil {
		fmt.Printf("Error creating symlinks: %v\n", err)
	}
}

// handleMemoizedCommand handles the execution of a memoized command
func handleMemoizedCommand(cmd string, args []string) {
	// The directory from which cash is operating might be determined by an environment variable,
	// or you might have a default location or strategy to find the configuration.
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

	// Here you would check if the command is memoized and either run it directly or fetch from cache.
	// For demonstration, just print the command and args.
	fmt.Printf("Executing memoized command: %s with args: %v\n", cmd, args)
	// Add logic to execute the command or retrieve its output from cache.
}

// handleMemoizedCommand handles executing and caching a memoized command
func (m *Cash) handleMemoizedCommand(cmd string, args []string) {
	hash := GenerateHash(cmd, args)
	stdoutPath, stderrPath := m.getCacheFilePaths(hash)

	if !m.cache.Exists(hash) {
		// Execute the command and capture stdout and stderr
		command := exec.Command(cmd, args...)
		stdoutFile, err := os.Create(stdoutPath)
		if err != nil {
			fmt.Println("Error creating stdout file:", err)
			return
		}
		defer stdoutFile.Close()

		stderrFile, err := os.Create(stderrPath)
		if err != nil {
			fmt.Println("Error creating stderr file:", err)
			return
		}
		defer stderrFile.Close()

		command.Stdout = stdoutFile
		command.Stderr = stderrFile

		if err := command.Run(); err != nil {
			fmt.Println("Error executing command:", err)
			return
		}

		// Add to cache
		m.cache.Add(hash, &CacheEntry{
			Hash:       hash,
			StdoutPath: stdoutPath,
			StderrPath: stderrPath,
		})
	} else {
		// Cache hit, output the cached result
		stdout, err := os.ReadFile(stdoutPath)
		if err != nil {
			fmt.Println("Error reading stdout from cache:", err)
			return
		}
		stderr, err := os.ReadFile(stderrPath)
		if err != nil {
			fmt.Println("Error reading stderr from cache:", err)
			return
		}
		fmt.Println("Cached stdout:", string(stdout))
		fmt.Println("Cached stderr:", string(stderr))
	}
}

func (m *Cash) getCacheFilePaths(hash string) (stdoutPath, stderrPath string) {
	baseDir := filepath.Join(m.CacheDir, "data", hash)
	stdoutPath = filepath.Join(baseDir, "stdout")
	stderrPath = filepath.Join(baseDir, "stderr")
	return
}

// You would need to initialize `m.cache` somewhere in your program, e.g., in `NewCash` or `Init`

// initializeEnv creates the cache directory and initializes the config file
func initializeEnv(cacheDir, configPath string) error {
	// Create the directory if it does not exist
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			return fmt.Errorf("failed to create cache directory: %w", err)
		}
	}

	// Check if the config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Initialize a default config
		defaultConfig := Config{
			MemoizeCommands: []string{"ls", "cat"}, // Add default commands to memoize
		}
		configFile, err := os.Create(configPath)
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
