package commands

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestPrintDryRunBanner(t *testing.T) {
	var buf bytes.Buffer
	printDryRunBanner(&buf)

	out := buf.String()
	if !strings.Contains(out, "DRY RUN") {
		t.Errorf("expected 'DRY RUN' in output, got: %s", out)
	}
}

func TestVerboseWriter_VerboseTrue(t *testing.T) {
	w := verboseWriter(true, false)
	if w != os.Stdout {
		t.Error("expected os.Stdout when verbose is true")
	}
}

func TestVerboseWriter_DryRunTrue(t *testing.T) {
	w := verboseWriter(false, true)
	if w != os.Stdout {
		t.Error("expected os.Stdout when dryRun is true")
	}
}

func TestVerboseWriter_BothFalse(t *testing.T) {
	w := verboseWriter(false, false)
	if w != io.Discard {
		t.Error("expected io.Discard when both are false")
	}
}
