package main

import (
	"os"
	"strings"

	"github.com/mad01/ralph/pkg/pipeutil"
)

// This is a simple example of a command-line tool that reads from stdin,
// converts the text to uppercase, and prints it to stdout.
// It demonstrates the use of the pipeutil package.
//
// To build and run:
// 1. cd pkg/pipeutil/example
// 2. go build -o uppercaser
// 3. echo "hello world" | ./uppercaser
func main() {
	binput, err := pipeutil.ReadAll()
	if err != nil {
		pipeutil.Errorf("failed to read from stdin: %v", err)
		os.Exit(pipeutil.ExitFailure)
	}

	input := string(binput)
	upper := strings.ToUpper(input)

	_, err = pipeutil.Println(upper)
	if err != nil {
		pipeutil.Errorf("failed to write to stdout: %v", err)
		os.Exit(pipeutil.ExitFailure)
	}

	// If successful, exit with success code (optional, as 0 is default)
	os.Exit(pipeutil.ExitSuccess)
}
