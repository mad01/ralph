" Minimal Neovim configuration
" Managed by ralph (editors recipe)

set number
set relativenumber
set cursorline
set scrolloff=8
set mouse=a
set encoding=utf-8

" Indentation
set tabstop=4 softtabstop=4 shiftwidth=4
set expandtab
set autoindent smartindent

" Searching
set incsearch
set hlsearch
set ignorecase smartcase

" UI
set termguicolors
syntax enable

" Leader key
let mapleader = " "

" Basic mappings
nnoremap <leader>w :w<CR>
nnoremap <leader>q :q<CR>
