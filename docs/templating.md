# Templating

ralph processes dotfiles marked with `is_template = true` using Go's `text/template` engine, allowing you to generate config files dynamically based on variables, environment, and platform.

## Basic syntax

Go templates use double curly braces for expressions:

| Syntax | Description |
|--------|-------------|
| `{{ .Variable }}` | Access a template variable |
| `{{ env "VAR" }}` | Read an environment variable |
| `{{ if .Condition }}...{{ end }}` | Conditional block |
| `{{ if eq (env "OS") "Darwin" }}...{{ else }}...{{ end }}` | Conditional with else |
| `{{/* comment */}}` | Template comment (not included in output) |
| `{{- .Var -}}` | Whitespace trimming (strips surrounding whitespace) |
| `{{ env "HOME" \| printf "%s/.local" }}` | Pipeline (pass value to next function) |

## Available data

When ralph processes a template, the following data is available:

### Template variables

All keys defined in `[template_variables]` in your config are available as top-level fields. For example, if your config contains:

```toml
[template_variables]
git_name = "Your Name"
git_email = "you@example.com"
```

Then your template can use `{{ .git_name }}` and `{{ .git_email }}` directly.

### RalphConfig object

The full configuration object is available as `.RalphConfig`:

| Field | Description |
|-------|-------------|
| `.RalphConfig` | The full ralph Config struct |
| `.RalphConfig.DotfilesRepoPath` | Path to the dotfiles repository |
| `.RalphConfig.TemplateVariables` | Map of all template variables |

### Custom functions

| Function | Description | Example |
|----------|-------------|---------|
| `env` | Read an environment variable | `{{ env "HOME" }}` |

### Built-in Go template functions

Go's `text/template` includes these comparison and logic functions:

| Function | Description | Example |
|----------|-------------|---------|
| `eq` | Equal | `{{ if eq .editor "vim" }}...{{ end }}` |
| `ne` | Not equal | `{{ if ne (env "USER") "root" }}...{{ end }}` |
| `lt` | Less than | `{{ if lt .count 10 }}...{{ end }}` |
| `gt` | Greater than | `{{ if gt .count 0 }}...{{ end }}` |
| `and` | Logical AND | `{{ if and .a .b }}...{{ end }}` |
| `or` | Logical OR | `{{ if or .a .b }}...{{ end }}` |
| `not` | Logical NOT | `{{ if not .disabled }}...{{ end }}` |

## Config setup

To use templating, mark a dotfile with `is_template = true` and define any variables you need:

```toml
[template_variables]
git_name = "Your Name"
git_email = "you@example.com"
editor = "nvim"

[dotfiles.gitconfig]
source = ".gitconfig.tmpl"
target = "~/.gitconfig"
is_template = true
```

The `source` file is processed through the template engine and the result is written to a temporary file, which is then symlinked to the `target` path.

## Examples

### .gitconfig with name and email

Template file (`.gitconfig.tmpl`):

```
[user]
    name = {{ .git_name }}
    email = {{ .git_email }}

[core]
    editor = {{ .editor }}
```

Config:

```toml
[template_variables]
git_name = "Jane Doe"
git_email = "jane@example.com"
editor = "nvim"

[dotfiles.gitconfig]
source = ".gitconfig.tmpl"
target = "~/.gitconfig"
is_template = true
```

Output (`~/.gitconfig`):

```
[user]
    name = Jane Doe
    email = jane@example.com

[core]
    editor = nvim
```

### OS-conditional aliases

Template file (`aliases.sh.tmpl`):

```bash
{{ if eq (env "OSTYPE") "darwin"* }}
alias ls="ls -G"
alias flush-dns="sudo dscacheutil -flushcache"
{{ else }}
alias ls="ls --color=auto"
alias open="xdg-open"
{{ end }}
```

A simpler approach using the `OS` environment variable:

```bash
{{ if eq (env "OS") "Darwin" }}
alias clipboard="pbcopy"
{{ else }}
alias clipboard="xclip -selection clipboard"
{{ end }}
```

### Host-conditional proxy settings

Template file (`.gitconfig.tmpl`):

```
[user]
    name = {{ .git_name }}
    email = {{ .git_email }}

{{ if eq (env "HOSTNAME") "work-laptop" }}
[http]
    proxy = http://corporate-proxy:8080

[user]
    email = {{ .work_email }}
{{ end }}
```

Config:

```toml
[template_variables]
git_name = "Jane Doe"
git_email = "jane@personal.com"
work_email = "jane@company.com"

[dotfiles.gitconfig]
source = ".gitconfig.tmpl"
target = "~/.gitconfig"
is_template = true
```

### Using pipelines

Pipelines pass the output of one expression as input to the next:

```
export LOCAL_BIN="{{ env "HOME" | printf "%s/.local/bin" }}"
```

This reads the `HOME` environment variable and formats it into a path.

### Whitespace control

Use `{{-` and `-}}` to trim whitespace around template expressions:

```
items: {{- range .items }}
  - {{ . }}
{{- end }}
```

The minus signs prevent extra blank lines in the output.

## How it works

When ralph encounters a dotfile with `is_template = true` during `ralph apply`:

1. It reads the source file from your dotfiles repository.
2. It parses the file as a Go template.
3. It builds a data map containing your `template_variables`, the `RalphConfig` object, and the `env` function.
4. It executes the template and writes the result to a temporary file.
5. It symlinks the target path to the processed temporary file.

Template processing errors (missing variables, syntax errors) are reported at apply time. Use `--dry-run` to validate templates without writing any files.

## Further reading

- [Getting started](getting-started.md) -- initial setup walkthrough
- [Configuration reference](configuration.md) -- full config.toml documentation
- Go `text/template` documentation: https://pkg.go.dev/text/template
