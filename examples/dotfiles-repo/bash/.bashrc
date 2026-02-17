# ~/.bashrc: executed by bash(1) for non-login shells.
# This is a templated file managed by ralph.

# If not running interactively, don't do anything
case $- in
    *i*) ;;
      *) return;;
esac

# Environment Variables
export EDITOR='{{ .editor | default "vim" }}'
export VISUAL='$EDITOR'
export LANG='en_US.UTF-8'
export LC_ALL='en_US.UTF-8'

# Set PATH, Manually adding common user paths
USER_PATHS=(
    "$HOME/bin"
    "$HOME/.local/bin"
    "/usr/local/bin"
    "/usr/local/sbin"
    # Add Go paths if Go is installed
    # You might want a more robust check for go installation
    "$HOME/go/bin"
)
for path_entry in "${USER_PATHS[@]}"; do
    if [ -d "$path_entry" ]; then
        PATH="$path_entry:$PATH"
    fi
done
export PATH

# Shell options
shopt -s autocd            # Enter a directory name to cd to it
shopt -s cdspell           # Correct minor spelling errors in cd
shopt -s checkwinsize      # Update window size after commands
shopt -s cmdhist           # Save multi-line commands as one history entry
shopt -s histappend        # Append to history, don't overwrite

# History settings
HISTCONTROL=ignoreboth:erasedups # Ignore duplicates and commands starting with space
HISTSIZE=10000
HISTFILESIZE=20000
PROMPT_COMMAND="history -a; history -c; history -r; $PROMPT_COMMAND" # Append before displaying prompt

# Completions
if ! shopt -oq posix; then
  if [ -f /usr/share/bash-completion/bash_completion ]; then
    . /usr/share/bash-completion/bash_completion
  elif [ -f /etc/bash_completion ]; then
    . /etc/bash_completion
  fi
fi

# Starship prompt init (if installed)
if command -v starship &> /dev/null; then
    eval "$(starship init bash)"
fi

# zoxide init (if installed)
if command -v zoxide &> /dev/null; then
    eval "$(zoxide init bash)"
fi

# fzf keybindings and fuzzy completion (if installed)
if command -v fzf &> /dev/null; then
  if [ -f ~/.fzf.bash ]; then
    source ~/.fzf.bash
  fi
fi

# Custom welcome message
echo "Welcome to Bash, {{ .name }}!"
echo "Ralph repo: {{ .RalphConfig.DotfilesRepoPath }}"

# Source aliases and functions managed by ralph (ralph will inject these)
# RALPH_SHELL_ALIASES_FILE will be set by ralph during 'apply'
if [ -n "$RALPH_SHELL_ALIASES_FILE" ] && [ -f "$RALPH_SHELL_ALIASES_FILE" ]; then
    source "$RALPH_SHELL_ALIASES_FILE"
fi

# RALPH_SHELL_FUNCTIONS_FILE will be set by ralph during 'apply'
if [ -n "$RALPH_SHELL_FUNCTIONS_FILE" ] && [ -f "$RALPH_SHELL_FUNCTIONS_FILE" ]; then
    source "$RALPH_SHELL_FUNCTIONS_FILE"
fi

# Source local/custom bash config if it exists
if [ -f ~/.bashrc_local ]; then
    source ~/.bashrc_local
fi 