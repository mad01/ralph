# Project Task List: ralph 🚀

This task list outlines the steps to build your Go-based CLI tool, `ralph`, for managing dotfiles, rc files, shell tools, and helper functions, inspired by Starship and using Cobra. Your project will be `github.com/mad01/ralph`.

## I. Core CLI & Project Setup 뼈대

* [x] **Initialize Go Module:**
    * Run `go mod init github.com/mad01/ralph` in your project's root directory.
* [x] **Project Structure:**
    * Define a clear directory structure. A common approach:
        * `cmd/ralph/` - Main application package for the `ralph` CLI.
        * `internal/` - Private application and library code (not intended for import by other projects).
            * `internal/config` - Configuration loading and management.
            * `internal/dotfile` - Logic for managing dotfiles (symlinking, templating).
            * `internal/shell` - Logic for interacting with shell rc files and managing functions/aliases.
            * `internal/tool` - Logic for managing external tools.
        * `pkg/` - Public library code, reusable by other projects.
            * `pkg/pipeutil` - Your helper package for stdin/stdout shell binaries.
        * `configs/examples/` - Example `ralph` configuration files.
        * `scripts/` - Helper scripts for building, testing, etc.
* [x] **Integrate Cobra CLI:**
    * Add Cobra: `go get -u github.com/spf13/cobra@latest`
    * Create the root command (`ralph`) in `cmd/ralph/main.go` and `cmd/ralph/commands/root.go`.
    * Define initial subcommands using Cobra (e.g., `init`, `apply`, `add`, `list`, `doctor`).
        * `cmd/ralph/commands/cmd_init.go`
        * `cmd/ralph/commands/cmd_apply.go`
        * `cmd/ralph/commands/cmd_add.go` (placeholder)
        * `cmd/ralph/commands/cmd_list.go`
        * `cmd/ralph/commands/cmd_doctor.go`
* [x] **Basic Logging/Output:**
    * Implement a simple logging mechanism for user feedback.
    * Consider using the standard `log` package initially, or a library like `github.com/charmbracelet/log` for more styled output later. (Used `fmt` and `log`, `charmbracelet/log` is a future enhancement)

## II. Configuration Management ⚙️

* [x] **Define Configuration Structure:**
    * The configuration file format will be **TOML**.
    * Define Go structs in `internal/config/types.go` to represent your configuration (e.g., `config.go`). This might include:
        * Path to the user's dotfiles source repository.
        * A list/map of dotfiles to manage (source relative to repo, target path on system).
        * Definitions for "standard tools" (name, install check command, config files).
        * Definitions for shell aliases and functions.
* [x] **Configuration Loading:**
    * Implement logic in `internal/config/load.go` to find and load the `config.toml` file.
    * Default location: `$XDG_CONFIG_HOME/ralph/config.toml` or `~/.config/ralph/config.toml`.
    * *Note: Users should be encouraged to keep their actual `config.toml` within their version-controlled dotfiles repository and symlink it to this default location. `ralph init` could assist with this.*
    * Consider using `github.com/spf13/viper` for configuration handling (it supports TOML well) or a dedicated TOML parser like `github.com/BurntSushi/toml`. (Used `BurntSushi/toml`)
* [x] **Default Configuration:**
    * Provide a way to generate a default `config.toml` (e.g., as part of the `ralph init` command). Store a template in `configs/examples/default.config.toml`. (`initCmd` generates a minimal one, can be improved to use the file or embed)
* [x] **Configuration Validation:**
    * Implement checks in `internal/config/validate.go` to ensure the loaded `config.toml` is valid (e.g., required fields are present, paths exist).

## III. Dotfile & RC File Management 📁

* [x] **Dotfile Specification in Config:**
    * Define how users specify which dotfiles to manage in `config.toml`. Example:
        ```toml
        [dotfiles.bashrc]
        source = ".bashrc" # Relative to dotfiles source repository
        target = "~/.bashrc"
        is_template = true # Optional

        [dotfiles.nvim_config]
        source = "nvim/init.vim"
        target = "~/.config/nvim/init.vim"
        ```
    * *Note: The `config.toml` itself, defining these mappings, is best version-controlled alongside the actual dotfiles in the user's source repository.*
* [x] **Symlinking Engine (`internal/dotfile/symlink.go`):**
    * Implement logic to create symbolic links from the source dotfiles repository to their target locations.
    * Handle path expansion (e.g., `~` to home directory).
    * Manage existing files at target locations: provide options (backup, overwrite, skip - configurable or via flags). (Basic backup implemented, flags are TODO)
* [x] **Templating (Optional but Recommended):**
    * Consider using Go's `text/template` package for dynamic content in dotfiles. (Implemented)
    * Allow users to define template variables in their `ralph` `config.toml` or use environment variables. (Basic env var access done, config variables TODO)
    * Process templates before symlinking. (Implemented)
* [x] **`ralph apply` Command (`cmd/ralph/commands/cmd_apply.go`):**
    * This command will be the core workhorse.
    * It should read the `config.toml`, then iterate through specified dotfiles and:
        * Process templates (if any).
        * Create/update symlinks.
        * Apply shell configurations (see next section).
* [x] **RC File Snippet Injection (`internal/shell/rc_manager.go`):**
    * Develop a robust method to add necessary sourcing lines or configurations to shell rc files (e.g., `~/.bashrc`, `~/.zshrc`, `~/.config/fish/config.fish`).
    * This is crucial for sourcing generated alias/function files or initializing tools.
    * Use markers to manage the block of code `ralph` adds, ensuring idempotency (e.g., `# BEGIN RALPH MANAGED BLOCK` ... `# END RALPH MANAGED BLOCK`).

## IV. Shell Tool & Function Management 🛠️

* [x] **Define "Standard Tools" in Config:**
    * Allow users to define tools they want `ralph` to manage or ensure are set up in `config.toml`.
    * Example in `config.toml`:
        ```toml
        [[tools]]
        name = "fzf"
        check_command = "command -v fzf" # How to check if installed
        install_hint = "Install fzf from [https://github.com/junegunn/fzf](https://github.com/junegunn/fzf)"
        # Optionally, specify config files for this tool that ralph should manage
        # config_files = [ { source = "fzf/.fzfrc", target = "~/.fzfrc" } ]
        ```
* [ ] **`ralph add tool <tool_name>` Command (Consider for later, focus on config-driven first):**
    * This could be a helper to add predefined tool configurations to the user's `ralph.toml`. (Placeholder command exists)
* [x] **Shell Function/Alias Management (`internal/shell/functions.go`):**
    * Allow users to define custom shell functions or aliases in `config.toml`.
        ```toml
        [shell.aliases]
        ll = "ls -alh"

        [shell.functions.myfunc]
        body = """
        echo "Hello from myfunc!"
        echo "Arguments: $@"
        """
        ```
    * `ralph apply` should generate a script (e.g., `~/.config/ralph/generated_aliases.sh`, `~/.config/ralph/generated_functions.sh`).
    * The RC file snippet injection (from Part III) should source these generated scripts.
* [x] **Go Function Integration (as `ralph` subcommands):**
    * Design how Go functions within `ralph` itself can be exposed as utility subcommands. (Cobra structure in place)
    * These are not shell functions but actual Go code executed by `ralph`.
    * Example: `ralph system-info` could be a subcommand written in Go.

## V. Helper Go Package for Custom Shell Binaries 📦 (`pkg/pipeutil`)

* [x] **Package Design (`pkg/pipeutil/pipeutil.go`):**
    * Create this new Go package: `github.com/mad01/ralph/pkg/pipeutil`.
* [x] **Stdin Handling:**
    * Provide utility functions to easily read all of `os.Stdin` (e.g., `ReadAll() ([]byte, error)`).
    * Consider line-by-line reading utilities (`Scanner() *bufio.Scanner`).
* [x] **Stdout Handling:**
    * Provide utility functions to easily write to `os.Stdout` (e.g., `Print(data []byte)`, `Println(s string)`).
* [x] **Stderr Handling:**
    * Provide utility functions for writing formatted error messages to `os.Stderr` (e.g., `Error(err error)`, `Errorf(format string, a ...any)`).
* [ ] **Simplified Argument Access (Optional):**
    * While the main `ralph` CLI uses Cobra, standalone binaries built with this `pipeutil` package might benefit from very simple flag/argument helpers if they are not complex enough to warrant Cobra themselves. Focus on stdin/stdout first.
* [x] **Standardized Exit Codes:**
    * Encourage or provide constants for common exit codes (e.g., `ExitSuccess = 0`, `ExitFailure = 1`).
* [x] **Example Usage/Templates:**
    * Create a clear example Go program in `pkg/pipeutil/example/` demonstrating how to use this package to build a simple filter/transformer.
    * Document how to compile and use these standalone binaries.

## VI. CLI Enhancements & Usability ✨

* [x] **`ralph init` Command (`cmd/ralph/commands/cmd_init.go`):**
    * Guides new users:
        * Creates a default `config.toml` in the appropriate config directory (e.g., `$XDG_CONFIG_HOME/ralph/config.toml`). (Minimal config created, see TODO for improvement)
        * Asks for the location of their dotfiles source repository (e.g., `~/.dotfiles_src`).
        * **Advises the user to commit their dotfiles source repository (including the `ralph` `config.toml` if they choose to place it there and symlink it) to version control (e.g., Git).**
        * **Optionally, offer to symlink `config.toml` if found in a conventional location within the user's dotfiles source repository to the expected `$XDG_CONFIG_HOME/ralph/config.toml`.** (Skipped for now, advised manually)
        * Provides instructions on next steps (e.g., "Populate your dotfiles repository, customize `config.toml`, then run `ralph apply`").
* [x] **`ralph list` Command (`cmd/ralph/commands/cmd_list.go`):**
    * Displays:
        * Managed dotfiles and their symlink status (e.g., linked, missing source, target conflict).
        * Configured tools and their detected status (installed/not installed). (Basic list, status check TODO)
        * Defined shell functions/aliases.
* [x] **`ralph doctor` Command (Status Check - `cmd/ralph/commands/cmd_doctor.go`):**
    * Checks the health of the `ralph` setup:
        * Verifies `config.toml` readability and validity.
        * Checks for broken symlinks.
        * Verifies if rc file snippets are correctly sourced (e.g., by checking for the presence of the generated function files in `PATH` or specific environment variables). (Basic RC block check, deeper check TODO)
* [x] **`--dry-run` Flag:**
    * Add a global persistent flag or a flag for commands like `apply` to show what changes *would* be made without actually making them. (Flag added, full implementation TODO)
* [x] **Colorized Output:**
    * Use a library (e.g., `github.com/fatih/color` or `github.com/charmbracelet/lipgloss`) for better visual feedback in the terminal. (Demonstrated, wider application TODO)
* [x] **Interactive Prompts (where appropriate):**
    * For actions like overwriting files, consider using a library like `github.com/AlecAivazis/survey/v2`. (Used in `initCmd`)

## VII. Testing & Documentation 🧪📄

* [x] **Unit Tests:**
    * Write unit tests for critical logic:
        * Config parsing (TOML) and validation (`internal/config`). (Started for `ExpandPath`)
        * Symlinking logic, especially edge cases (`internal/dotfile`). (TODO)
        * Template execution. (TODO)
        * `pkg/pipeutil` functions. (TODO)
* [ ] **Integration Tests (Basic):**
    * Consider simple integration tests for CLI commands.
    * This might involve setting up a temporary home directory, running `ralph` commands using `os/exec`, and then asserting file system state or command output.
* [x] **README.md:**
    * Create a comprehensive `README.md` for `github.com/mad01/ralph`:
        * Project overview, philosophy, and goals.
        * Installation instructions (Go install, binaries from releases).
        * Detailed configuration guide with examples (using TOML).
        * Best practices for version controlling the dotfiles repository and the `config.toml`.
        * Usage examples for all commands and flags.
        * How to use the `pkg/pipeutil` package to build custom tools.
* [ ] **Contributing Guidelines (CONTRIBUTING.md):**
    * If you plan for others to contribute (coding style, PR process).
* [ ] **GoDoc:**
    * Write clear GoDoc comments for all public functions and packages, especially for `pkg/pipeutil`. (Partially done, needs thorough pass)

## VIII. Build & Release 🏗️

* [x] **Makefile / Build Scripts (`scripts/build.sh`, `Makefile`):**
    * Create scripts or a `Makefile` for common development tasks:
        * `make build`
        * `make test`
        * `make lint` (using `golangci-lint`)
        * `make format` (using `gofmt` or `goimports`)
* [ ] **Cross-Compilation (Optional but good for distribution):**
    * Set up `goreleaser` or use Go's built-in cross-compilation features to build binaries for different OS/architectures (Linux, macOS, Windows).
* [ ] **GitHub Releases:**
    * Set up GitHub Actions to automate building releases when you tag a new version.
    * `goreleaser` is excellent for this.

## IX. Starship Inspiration Points (Keep in Mind) ✨

* [x] **Modularity:** Design `ralph` so that different functionalities (dotfile linking, shell setup, tool management) are as decoupled as possible.
* [x] **Configuration over Code:** Emphasize defining behavior through the `config.toml` file.
* [ ] **Speed & Efficiency:** Keep performance in mind, especially for any part of `ralph` that might be involved in shell startup (e.g., sourcing generated files). (Ongoing consideration)
* [x] **Clear Diagnostics:** When things go wrong, provide helpful, actionable error messages (the `ralph doctor` command is key here).
* [x] **Extensibility (Future):** Think about how users might eventually be able to add their own "plugin" behaviors (though this is likely beyond the initial scope).
  * [x] **Lifecycle Hook System:** Implement a hooks system that allows users to run custom scripts at key points in the ralph lifecycle.
  * [ ] **Custom Template Functions:** Extend the templating system to allow users to register custom template functions.
  * [ ] **Platform-Specific Configurations:** Allow users to define platform-specific variations of dotfiles.
  * [ ] **Plugin System:** Design a more formal plugin system where Go packages can extend ralph's functionality.

---
## New TODOs / Refinements:

*   [ ] **`ralph init` Improvement:** Use `go:embed` for `configs/examples/default.config.toml` to make `initCmd` more robust in creating the initial config file.
*   [ ] **Tool Status Check:** Implement the logic to run `tool.CheckCommand` in `cmd_list.go` and `cmd_doctor.go`.
*   [ ] **`applyCmd` Flags:** Implement `--overwrite` and `--skip` flags for `ralph apply` to control symlink behavior.
*   [ ] **Full `--dry-run` Implementation:** Propagate `dryRun` status to file operation functions (symlinking, writing files, RC management) so they only print intended actions.
*   [ ] **Wider Colorization:** Apply `fatih/color` more broadly across CLI output for better visual feedback.
*   [ ] **Template Variables from Config:** Allow users to define variables in `config.toml` for use in templates.
*   [ ] **Unit Tests Expansion:** Add more unit tests for:
    *   `internal/config` (parsing, full validation).
    *   `internal/dotfile` (symlinking edge cases, templating).
    *   `internal/shell` (RC management, alias/function generation).
    *   `pkg/pipeutil`.
*   [ ] **`ralph doctor` RC Sourcing Check:** Enhance the check for RC file sourcing to verify that the sourced script files (aliases, functions) actually exist.
*   [ ] **Error Handling & User Feedback:** Review and improve error messages and user feedback across all commands.
*   [ ] **`ralph add tool` Implementation:** Flesh out the `ralph add tool <tool_name>` command.
*   [ ] **CONTRIBUTING.md:** Create if project aims for external contributions.
*   [ ] **LICENSE file:** Add a license file (e.g., MIT).
*   [ ] **Further Extensibility Features:**
    * [ ] **Enhanced Hook System:** Add more hook points (e.g., pre/post shell config, pre/post tool check).
    * [ ] **Custom Template Functions:** Allow users to define shell scripts that can be called as functions within templates.
    * [ ] **Platform Detection & Conditional Configurations:** Add OS/platform detection to enable conditional dotfile handling.
    * [ ] **Remote Scripts Support:** Add capability to download and execute remote hook scripts (with appropriate security measures).
    * [ ] **Hook Timeouts and Error Handling:** Implement timeout and retry mechanisms for hooks.
*   [ ] **Sandbox Environment Updates:** Extend the sandbox environment to include examples of the hook system in action.
*   [ ] **Documentation for Hooks:** Add comprehensive documentation for the hook system with examples.
---

This list is quite detailed to give you a good starting point. Remember to break these down into smaller, manageable tasks as you go. Good luck with building `ralph`!