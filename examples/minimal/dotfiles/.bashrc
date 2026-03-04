# .bashrc - managed by ralph

# If not running interactively, don't do anything
[[ $- != *i* ]] && return

# History settings
HISTSIZE=5000
HISTFILESIZE=10000
HISTCONTROL=ignoreboth

# Basic exports
export EDITOR="vim"
export LANG="en_US.UTF-8"

# Prompt
PS1='\[\e[32m\]\u@\h\[\e[0m\]:\[\e[34m\]\w\[\e[0m\]\$ '

# Load local overrides if present
if [ -f ~/.bashrc.local ]; then
    source ~/.bashrc.local
fi
