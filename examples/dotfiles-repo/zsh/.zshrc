# ~/.zshrc: executed by zsh(1) for non-login shells.
# This is a templated file managed by ralph.

# Environment Variables
export EDITOR='{{ .editor | default "vim" }}'
export VISUAL='$EDITOR'
export LANG='en_US.UTF-8'
export LC_ALL='en_US.UTF-8'

# Set PATH, Manually adding common user paths
# Zsh uses `typeset -U path` which handles uniqueness and ordering automatically.
# Ensure these are added early for tools like antibody, asdf, etc.
USER_PATHS=(
    $HOME/bin
    $HOME/.local/bin
    /usr/local/bin
    /usr/local/sbin
    # Add Go paths if Go is installed
    $HOME/go/bin
)
path=($USER_PATHS $path)

# Zsh options (setopt)
setopt autocd              # Enter a directory name to cd to it
setopt correct             # Correct commands
setopt extendedglob          # Use extended globbing features
setopt nomatch             # Don't error if glob finds no match
setopt notify              # Report status of background jobs immediately
setopt promptsubst         # Allow parameter expansion in prompts

# History settings
HISTFILE=~/.zsh_history
HISTSIZE=10000
SAVEHIST=10000
setopt appendhistory       # Append to history file
setopt sharehistory        # Share history between all sessions
setopt histignorealldups   # If a new command is a duplicate, remove the older one
setopt histignorespace     # Don't save commands starting with a space
setopt incappendhistory    # Save history entries as they are entered

# Keybindings (use `bindkey`)
# Example: Ctrl+R for fzf history search
# if command -v fzf &> /dev/null; then
#   bindkey '^R' fzf-history-widget
# fi

# Completions
# Ensure `compinit` is loaded. Often done by plugin managers or frameworks.
# If not using a framework, you might need:
autoload -Uz compinit
if [ $(date +%j) != $(stat -c %z ~/.zcompdump | date +%j) ]; then
  compinit
else
  compinit -C
fi

# Starship prompt init (if installed)
if command -v starship &> /dev/null; then
    eval "$(starship init zsh)"
fi

# zoxide init (if installed)
if command -v zoxide &> /dev/null; then
    eval "$(zoxide init zsh)"
fi

# fzf keybindings and fuzzy completion (if installed)
if command -v fzf &> /dev/null; then
  if [ -f ~/.fzf.zsh ]; then
    source ~/.fzf.zsh
  fi
fi

# Custom welcome message
echo "Welcome to Zsh, {{ .name }}!"
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

# Source local/custom zsh config if it exists
if [ -f ~/.zshrc_local ]; then
    source ~/.zshrc_local
fi 