" Minimal Neovim init.vim
" This file is managed by ralph.

" Basic Settings
set number                     " Show line numbers
set relativenumber             " Show relative line numbers
set cursorline                 " Highlight the current line
set scrolloff=8                " Keep 8 lines visible above/below cursor
set sidescrolloff=8            " Keep 8 columns visible left/right of cursor
set mouse=a                    " Enable mouse support
set encoding=utf-8
set fileformats=unix,dos,mac   " Prefer Unix line endings

" Indentation
set tabstop=4 softtabstop=4 shiftwidth=4
set expandtab                  " Use spaces instead of tabs
set autoindent smartindent

" Searching
set incsearch                  " Incremental search
set hlsearch                   " Highlight search results
set ignorecase smartcase       " Ignore case unless uppercase is used

" UI
set termguicolors              " Enable true colors
syntax enable                  " Enable syntax highlighting
colorscheme desert             " Default colorscheme, change as needed

" Files and Buffers
set hidden                     " Allow hidden buffers
set confirm                    " Ask for confirmation before quitting with unsaved changes
set autoread                   " Automatically re-read files changed outside Vim

" Backup and Swap files (consider placing these in a dedicated directory)
set backupdir=~/.config/nvim/backup//
set directory=~/.config/nvim/swap//
set undodir=~/.config/nvim/undo//
set undofile                   " Persistent undo

" Leader key
let mapleader = " "            " Set leader key to Space

" Example plugin management (using vim-plug)
" Ensure vim-plug is installed: 
" curl -fLo ~/.local/share/nvim/site/autoload/plug.vim --create-dirs \
"    https://raw.githubusercontent.com/junegunn/vim-plug/master/plug.vim

" call plug#begin('~/.local/share/nvim/plugged')

" " Example plugins
" Plug 'tpope/vim-sensible'       " Sensible Vim defaults
" Plug 'tpope/vim-fugitive'       " Git integration
" Plug 'preservim/nerdtree'       " File explorer
" Plug 'junegunn/fzf.vim'         " fzf integration
" Plug 'nvim-treesitter/nvim-treesitter', {'do': ':TSUpdate'} " Treesitter for syntax highlighting
" Plug 'catppuccin/nvim', { 'as': 'catppuccin' } " Catppuccin theme (Mocha by default)

" call plug#end()

" If using vim-plug and Catppuccin, uncomment this to set theme:
" if exists(":Catppuccin"))
"   colorscheme catppuccin
" endif

" Basic mappings
nnoremap <leader>w :w<CR>
nnoremap <leader>q :q<CR>
nnoremap <leader>wq :wq<CR>

" NERDTree toggle example (if installed)
" nnoremap <leader>n :NERDTreeToggle<CR>

" fzf mappings (if installed)
" nnoremap <leader>ff :Files<CR>
" nnoremap <leader>fg :GFiles<CR>
" nnoremap <leader>fb :Buffers<CR>
" nnoremap <leader>fh :History<CR>

" You can add more language-specific settings, LSP configurations, etc., here.

" Load local/custom nvim config if it exists
if filereadable(expand("~/.config/nvim/init_local.vim"))
  source ~/.config/nvim/init_local.vim
endif 