# pipeutil

A utility package for writing Go programs that work as shell filters or stream processors.

## Import Path

```go
import "github.com/mad01/ralph/pkg/pipeutil"
```

## Functions

| Function | Signature | Description |
|----------|-----------|-------------|
| `ReadAll` | `func ReadAll() ([]byte, error)` | Reads all data from `os.Stdin`. |
| `Scanner` | `func Scanner() *bufio.Scanner` | Returns a `bufio.Scanner` for line-by-line reading of `os.Stdin`. |
| `Print` | `func Print(data []byte) (int, error)` | Writes raw bytes to `os.Stdout`. Does not append a newline. |
| `Println` | `func Println(s string) (int, error)` | Writes a string followed by a newline to `os.Stdout`. |
| `Error` | `func Error(err error)` | Prints an error message to `os.Stderr` in the format `Error: <message>`. No-op if `err` is `nil`. |
| `Errorf` | `func Errorf(format string, a ...interface{})` | Prints a formatted error message to `os.Stderr` in the format `Error: <formatted message>`. |

## Constants

| Constant | Value | Description |
|----------|-------|-------------|
| `ExitSuccess` | `0` | Standard success exit code. |
| `ExitFailure` | `1` | Standard failure exit code. |

## Example: Uppercase Filter

A complete program that reads stdin, converts the text to uppercase, and writes it to stdout.

```go
package main

import (
	"os"
	"strings"

	"github.com/mad01/ralph/pkg/pipeutil"
)

func main() {
	binput, err := pipeutil.ReadAll()
	if err != nil {
		pipeutil.Errorf("failed to read from stdin: %v", err)
		os.Exit(pipeutil.ExitFailure)
	}

	upper := strings.ToUpper(string(binput))

	_, err = pipeutil.Println(upper)
	if err != nil {
		pipeutil.Errorf("failed to write to stdout: %v", err)
		os.Exit(pipeutil.ExitFailure)
	}

	os.Exit(pipeutil.ExitSuccess)
}
```

Build and run:

```bash
go build -o uppercaser .
echo "hello world" | ./uppercaser
# Output: HELLO WORLD
```

## Example: Line-by-Line Processing with Scanner

For large inputs or streaming data, use `Scanner` instead of `ReadAll`:

```go
package main

import (
	"os"
	"strings"

	"github.com/mad01/ralph/pkg/pipeutil"
)

func main() {
	scanner := pipeutil.Scanner()
	for scanner.Scan() {
		line := scanner.Text()
		_, err := pipeutil.Println(strings.TrimSpace(line))
		if err != nil {
			pipeutil.Errorf("write failed: %v", err)
			os.Exit(pipeutil.ExitFailure)
		}
	}
	if err := scanner.Err(); err != nil {
		pipeutil.Error(err)
		os.Exit(pipeutil.ExitFailure)
	}
}
```

## Usage with Ralph Build Hooks

You can use pipeutil to build custom shell tools and manage them with ralph's build hooks or packages. Place the tool source in your dotfiles repo and add a build hook to compile it:

```toml
[hooks.builds.uppercaser]
commands = ["go build -o ~/.local/bin/uppercaser ."]
working_dir = "~/dotfiles/tools/uppercaser"
run = "once"
```

Or manage it as a package for change-tracked rebuilds with `ralph update`:

```toml
[packages.uppercaser]
source = "local"
working_dir = "~/dotfiles/tools/uppercaser"
build = ["go build -o ~/.local/bin/uppercaser ."]
```

See [commands](commands.md) for details on `ralph apply --build` and `ralph update`, and [configuration](configuration.md) for the full `[hooks.builds]` and `[packages]` schemas.
