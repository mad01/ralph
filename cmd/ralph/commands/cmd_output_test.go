package commands

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/mad01/ralph/internal/report"
)

// captureStdout runs fn with os.Stdout redirected to a pipe and returns what
// was written. It restores os.Stdout before returning.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	_ = w.Close()
	os.Stdout = orig
	return <-done
}

// withOutputFormat sets the global output format for the duration of fn.
func withOutputFormat(format string, fn func()) {
	prev := outputFormat
	outputFormat = format
	defer func() { outputFormat = prev }()
	fn()
}

func sampleReport() *report.Report {
	r := &report.Report{Command: "up"}
	p := r.AddPhase("Dotfiles")
	p.AddOK("bashrc", "linked")
	p.AddFail("broken", "could not link", errors.New("boom"))
	return r
}

func TestFinishReportJSONIsPureJSON(t *testing.T) {
	var out string
	withOutputFormat("json", func() {
		out = captureStdout(t, func() {
			finishReport(sampleReport(), nil, false, false)
		})
	})

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("finishReport(json) did not emit pure JSON: %v\n%s", err, out)
	}
	if parsed["command"] != "up" {
		t.Errorf("command = %v, want up", parsed["command"])
	}
	if parsed["exit_code"].(float64) != 1 {
		t.Errorf("exit_code = %v, want 1", parsed["exit_code"])
	}
}

func TestFinishDoctorJSONIsPureJSON(t *testing.T) {
	var out string
	withOutputFormat("json", func() {
		out = captureStdout(t, func() {
			finishDoctor(sampleReport(), false)
		})
	})
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("finishDoctor(json) did not emit pure JSON: %v\n%s", err, out)
	}
}

func TestFinishReportTextIsNotJSON(t *testing.T) {
	var out string
	withOutputFormat("text", func() {
		out = captureStdout(t, func() {
			finishReport(sampleReport(), nil, false, false)
		})
	})
	// Text mode prints the human summary line, never a JSON document.
	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Errorf("text mode emitted JSON-looking output:\n%s", out)
	}
	if !strings.Contains(out, "Complete —") {
		t.Errorf("text mode missing 'Complete —' summary line:\n%s", out)
	}
}
