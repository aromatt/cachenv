package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

const (
	CONFIG_NAME        = "config.yaml"
	LINKS_IN_PATH_NAME = "links-in-path"
	LINKS_TO_REAL_NAME = "links-to-real"
)

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

func (c *Cachenv) LoadConfig() error {
	configFile, err := os.Open(c.ConfigPath)
	if err != nil {
		return fmt.Errorf("failed to open config file: %w", err)
	}
	defer configFile.Close()

	decoder := yaml.NewDecoder(configFile)
	if err := decoder.Decode(&c.Config); err != nil {
		return fmt.Errorf("failed to decode config: %w", err)
	}

	return nil
}

func (c *Cachenv) IsCommandMemoized(command string) bool {
	_, ok := c.Config.Commands[command]
	return ok
}

// Directory containing symlinks cmd -> cachenv executable
func (c *Cachenv) DirLinksInPath() string {
	return filepath.Join(c.Dir, LINKS_IN_PATH_NAME)
}

// Directory containing symlinks cmd -> real cmd
func (c *Cachenv) DirLinksToReal() string {
	return filepath.Join(c.Dir, LINKS_TO_REAL_NAME)
}

// Relative symlink target for links pointing from DirLinksInPath -> DirLinksToReal
func (c *Cachenv) LinkToRealRelative(cmdName string) string {
	return filepath.Join("..", LINKS_TO_REAL_NAME, cmdName)
}

// Name of link which appears in $PATH due to activate script
func (c *Cachenv) LinkInPath(cmd string) string {
	return filepath.Join(c.DirLinksInPath(), cmd)
}

// Name of link that points to the real cmd
func (c *Cachenv) LinkToReal(cmd string) string {
	return filepath.Join(c.DirLinksToReal(), cmd)
}

// Path to the real cachenv executable
func (c *Cachenv) LinkToRealCachenv() string {
	return filepath.Join(c.DirLinksToReal(), "cachenv")
}

func (c *Cachenv) CreateLinksDirs() error {
	for _, dir := range []string{c.DirLinksInPath(), c.DirLinksToReal()} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create %s, directory: %w", dir, err)
		}
	}
	return nil
}

// Creates a symlink to the cachenv executable. We can't just use
// LinkCommand("cachenv") because while activated, 'cachenv' is a shell
// function and won't be findable by LinkCommand().
func (c *Cachenv) CreateCachenvLink() error {
	cachenvExecPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get cachenv executable path: %w", err)
	}
	if err := os.Symlink(cachenvExecPath, c.LinkToRealCachenv()); err != nil {
		return fmt.Errorf("failed to create symlink to real cachenv: %w", err)
	}
	return nil
}

func (c *Cachenv) Init() error {
	if err := c.InitializeEnv(); err != nil {
		return err
	}

	if err := c.LoadConfig(); err != nil {
		return err
	}

	if err := c.CreateActivateScript(); err != nil {
		return err
	}

	if err := c.CreateLinksDirs(); err != nil {
		return err
	}

	if err := c.CreateCachenvLink(); err != nil {
		return err
	}

	return nil
}

func (c *Cachenv) LinkCommand(cmd string) error {
	linkInPath := c.LinkInPath(cmd)
	linkToReal := c.LinkToReal(cmd)

	// Remove existing symlinks for cmd if they exist.
	for _, link := range []string{linkInPath, linkToReal} {
		if _, err := os.Lstat(link); err == nil {
			if err := os.Remove(link); err != nil {
				return fmt.Errorf("failed to remove existing symlink for %s: %w", cmd, err)
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("failed to stat symlink for %s: %w", cmd, err)
		}
	}

	// Order matters here!

	// 1. Create symlink cmd -> real cmd (via exec.LookPath), to avoid recursive
	// cachenv invocations
	if realPath, err := exec.LookPath(cmd); err == nil {
		if err := os.Symlink(realPath, linkToReal); err != nil {
			return fmt.Errorf("failed to create symlink for %s: %w", cmd, err)
		}
	} else {
		return fmt.Errorf("failed to find real path for %s: %w", cmd, err)
	}

	// 2. Create symlink <cmd in $PATH> -> cachenv, so we can intercept
	// invocations
	// Note: we use a relative link target to make envs more easily portable
	if err := os.Symlink(c.LinkToRealRelative("cachenv"), linkInPath); err != nil {
		return fmt.Errorf("failed to create symlink for %s: %w", cmd, err)
	}

	fmt.Printf("Created symlink for %s\n", cmd)
	return nil
}

// LinkCommands synchronizes actual symlinks with config
func (c *Cachenv) LinkCommands() error {
	// Iterate over commands from the config file
	for cmd := range c.Config.Commands {
		c.LinkCommand(cmd)
	}

	// Iterate over symlinks to delete any that are not in the config
	entries, err := os.ReadDir(c.DirLinksInPath())
	if err != nil {
		return fmt.Errorf("failed to read bin directory: %w", err)
	}
	for _, entry := range entries {
		if entry.Name() == "activate" {
			continue
		}
		if _, ok := c.Config.Commands[entry.Name()]; !ok {
			symlinkPath := filepath.Join(c.DirLinksInPath(), entry.Name())
			if err := os.Remove(symlinkPath); err != nil {
				return fmt.Errorf("failed to remove symlink for %s: %w", entry.Name(), err)
			}
			fmt.Printf("Removed symlink for %s\n", entry.Name())
		}
	}

	return nil
}

func (c *Cachenv) CreateActivateScript() error {
	// Define the path to the activate script within the bin directory
	activateScriptPath := filepath.Join(c.Dir, "activate")

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
export _CACHENV_EXECUTABLE="${CACHENV}/%s/cachenv"

export PATH="$CACHENV/%s:$PATH"

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
`, LINKS_TO_REAL_NAME, LINKS_IN_PATH_NAME)

	// Ensure the bin directory exists
	if err := os.MkdirAll(c.DirLinksInPath(), 0755); err != nil {
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
	// Create the c.Dir if it does not exist
	if _, err := os.Stat(c.Dir); os.IsNotExist(err) {
		if err := os.MkdirAll(c.Dir, 0755); err != nil {
			return fmt.Errorf("failed to create cache directory: %w", err)
		}
	}

	// Create the config file if it does not exist
	if _, err := os.Stat(c.ConfigPath); os.IsNotExist(err) {
		defaultConfig := Config{
			Commands: make(map[string]CommandConfig, 0),
			Cache: CacheConfig{
				MaxEntries: 1000, // TODO make configurable
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

type ExecResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

func (c *Cachenv) PrepareRealCommand(cmdName string, args ...string) *exec.Cmd {
	return exec.Command(filepath.Join(c.DirLinksToReal(), cmdName), args...)
}

func (c *Cachenv) ExecuteRealCommand(cmdName string, args ...string) (ExecResult, error) {
	var exitCode int
	var stdoutBuf, stderrBuf bytes.Buffer

	cmd := c.PrepareRealCommand(cmdName, args...)

	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			return ExecResult{}, fmt.Errorf("Error executing command: %v\n", err)
		}
	} else {
		exitCode = cmd.ProcessState.ExitCode()
	}

	return ExecResult{
		Stdout:   stdoutBuf.Bytes(),
		Stderr:   stderrBuf.Bytes(),
		ExitCode: exitCode,
	}, nil
}

func (c *Cachenv) HandleMemoizedCommand(cmd string, args []string) int {
	key := KeyFrom(cmd, args)
	var result ExecResult
	var err error

	if c.Store.Exists(key) {
		result, err = c.Store.ReadFromCache(key)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read from cache: %v\n", err)
			return 1
		}
	} else {
		result, err = c.ExecuteRealCommand(cmd, args...)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error executing command: %v\n", err)
			return 1
		}

		err = c.Store.WriteToCache(key, result)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write to cache: %v\n", err)
			return 1
		}
	}

	fmt.Fprint(os.Stdout, string(result.Stdout))
	fmt.Fprint(os.Stderr, string(result.Stderr))
	return result.ExitCode
}

func main() {
	// This program is used both for controlling cachenv (e.g. `cachenv init`) and
	// for intercepting memoized commands. Use $0 to determine which is
	// happening.
	invokedCmd := filepath.Base(os.Args[0])
	exitCode := 0
	switch invokedCmd {
	case "cachenv":
		if len(os.Args) < 2 {
			fmt.Println("Usage: cachenv <command> [arguments]")
			return
		}
		exitCode = handleCachenvSubcommand(os.Args[1], os.Args[2:])
	default:
		exitCode = handleMemoizedCommand(invokedCmd, os.Args[1:])
	}
	os.Exit(exitCode)
}

func handleCachenvSubcommand(subcommand string, args []string) int {
	switch subcommand {
	case "init":
		return handleInit(args)
	case "link":
		return handleLink(args)
	case "add":
		return handleAdd(args)
	case "key":
		return handleKey(args)
	case "touch":
		return handleTouch(args)
	case "diff":
		return handleDiff(args)
	default:
		fmt.Println("Invalid command. Available commands are: init, link, add")
		return 1
	}
}

func handleInit(args []string) int {
	if len(args) < 1 {
		fmt.Println("Usage: cachenv init <DIR>")
		return 1
	}
	dir := args[0]

	cachenv := loadCachenvFromDir(dir)
	if err := cachenv.Init(); err != nil {
		fmt.Printf("Error initializing cachenv: %v\n", err)
		return 1
	}

	return 0
}

// - `cachenv link` while activated
func handleLink(args []string) int {
	c, err := loadActiveCachenv()
	if err != nil {
		fmt.Printf("Error loading active cachenv: %v\n", err)
		return 1
	}

	if err := c.LinkCommands(); err != nil {
		fmt.Printf("Error creating symlinks: %v\n", err)
		return 1
	}

	return 0
}

func handleAdd(args []string) int {
	if len(args) < 1 {
		fmt.Println("Usage: cachenv add <command>")
		return 1
	}

	c, err := loadActiveCachenv()
	if err != nil {
		fmt.Printf("Error loading active cachenv: %v\n", err)
		return 1
	}

	cmdName := args[0]
	if c.IsCommandMemoized(cmdName) {
		fmt.Printf("Command '%s' is already memoized.\n", cmdName)
		return 1
	}

	c.Config.Commands[cmdName] = CommandConfig{}
	configFile, err := os.Create(c.ConfigPath)
	if err != nil {
		fmt.Printf("Error opening config file: %v\n", err)
		return 1
	}
	defer configFile.Close()

	encoder := yaml.NewEncoder(configFile)
	if err := encoder.Encode(c.Config); err != nil {
		fmt.Printf("Error encoding config: %v\n", err)
		return 1
	}

	fmt.Printf("Command '%s' added to memoized commands.\n", cmdName)

	if err := c.LinkCommand(cmdName); err != nil {
		fmt.Printf("Error creating symlinks: %v\n", err)
		return 1
	}

	return 0
}

// Returns the hash ID for the provided cached command (+ args)
func handleKey(args []string) int {
	if len(args) < 1 {
		fmt.Println("Usage: cachenv key <command>")
		return 1
	}
	fmt.Println(KeyFrom(args[0], args[1:]).Hash)
	return 0
}

// Like touch(1), creates an empty cache entry, or updates the timestamp of an
// existing on cache entry.
// TODO: actually do the latter
func handleTouch(args []string) int {
	if len(args) < 1 {
		fmt.Println("Usage: cachenv touch <command>")
		return 1
	}

	c, err := loadActiveCachenv()
	if err != nil {
		fmt.Printf("Error loading active cachenv: %v\n", err)
		return 1
	}

	command := args[0]
	if !c.IsCommandMemoized(command) {
		fmt.Printf("Command '%s' is not memoized.\n", command)
		return 1
	}

	key := KeyFrom(command, args[1:])
	err = c.Store.WriteToCache(key, ExecResult{})
	fmt.Println(key.Hash)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write to cache: %v\n", err)
		return 1
	}
	return 0
}

// Run the real command and print `diff -u <cached> <actual>`
func handleDiff(args []string) int {
	if len(args) < 1 {
		fmt.Println("Usage: cachenv diff <command>")
		return 1
	}

	c, err := loadActiveCachenv()
	if err != nil {
		fmt.Printf("Error loading active cachenv: %v\n", err)
		return 1
	}

	// Run the real command and pipe its output to diff
	cmd := c.PrepareRealCommand(args[0], args[1:]...)
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating pipe for '%s': %v\n", args[0], err)
		return 1
	}
	cmd.Stderr = os.Stderr

	key := KeyFrom(args[0], args[1:])
	diffCmd := exec.Command("diff", c.Store.stdoutPath(key), "-")

	diffCmd.Stdin = stdoutPipe
	diffCmd.Stdout = os.Stdout
	diffCmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting '%s': %v\n", args[0], err)
		return 1
	}

	if err := diffCmd.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			return exitError.ExitCode()
		} else {
			fmt.Fprintf(os.Stderr, "Error running diff: %v\n", err)
			return 1
		}
	}

	return 0
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

func loadActiveCachenv() (*Cachenv, error) {
	dir, err := getActiveCachenvDir()
	if err != nil {
		return nil, fmt.Errorf("failed to find active cachenv: %v\n", err)
	}
	c := loadCachenvFromDir(dir)
	if err := c.LoadConfig(); err != nil {
		return nil, fmt.Errorf("failed to load config: %v\n", err)
	}
	return c, nil
}

// HandleMemoizedCommand handles the execution of a memoized command
func handleMemoizedCommand(cmd string, args []string) int {
	c, err := loadActiveCachenv()
	if err != nil {
		fmt.Printf("Error loading active cachenv: %v\n", err)
	}
	return c.HandleMemoizedCommand(cmd, args)
}
