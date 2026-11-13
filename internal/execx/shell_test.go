package execx

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"
	"time"
)

// stderrOf runs fn with os.Stderr redirected and returns what it wrote.
func stderrOf(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	defer func() { os.Stderr = orig }()
	fn()
	_ = w.Close()
	return <-done
}

func TestShell_QuietReplaysKeptStderrOnFailure(t *testing.T) {
	var out bytes.Buffer
	sh := &Shell{Out: &out}
	var firstErr, secondErr error
	got := stderrOf(t, func() {
		firstErr = sh.Run(context.Background(), "-c", "echo one; echo warn-one >&2")
		secondErr = sh.Run(context.Background(), "-c", "echo fail-two >&2; exit 3")
	})
	if firstErr != nil || secondErr == nil {
		t.Fatalf("Run errors = %v, %v, want nil then a failure", firstErr, secondErr)
	}
	if got != "warn-one\nfail-two\n" {
		t.Errorf("stderr = %q, want both steps' stderr replayed once", got)
	}
	if out.String() != "one\n" {
		t.Errorf("stdout = %q, want %q", out.String(), "one\n")
	}
}

func TestShell_QuietSuccessWritesNoStderr(t *testing.T) {
	sh := &Shell{Out: io.Discard}
	got := stderrOf(t, func() {
		if err := sh.Run(context.Background(), "-c", "echo noise >&2"); err != nil {
			t.Errorf("Run() error = %v", err)
		}
	})
	if got != "" {
		t.Errorf("stderr = %q, want nothing on success", got)
	}
}

func TestShell_TimeoutReplaysNothing(t *testing.T) {
	sh := &Shell{Out: io.Discard}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	var err error
	got := stderrOf(t, func() {
		err = sh.Run(ctx, "-c", "echo before >&2; sleep 5")
	})
	if err == nil || !TimedOut(ctx) {
		t.Fatalf("Run() error = %v, timed out = %v, want a timeout", err, TimedOut(ctx))
	}
	if got != "" {
		t.Errorf("stderr = %q, want nothing replayed on a timeout", got)
	}
}

func TestShell_RunsInDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/marker", []byte("here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	sh := &Shell{Out: &out, Dir: dir, Verbose: true}
	if err := sh.Run(context.Background(), "-c", "cat marker"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if out.String() != "here\n" {
		t.Errorf("stdout = %q, want the marker read from Dir", out.String())
	}
}
