package gitutil

import (
	"slices"
	"testing"
)

func TestCloneArgs_InsertsSeparatorBeforePositionals(t *testing.T) {
	args := cloneArgs("-oProxyCommand=evil", "/tmp/t", "main")
	// "--" must appear before the URL so it cannot be parsed as an option.
	sep := slices.Index(args, "--")
	url := slices.Index(args, "-oProxyCommand=evil")
	if sep == -1 || url == -1 || sep > url {
		t.Errorf("expected -- before URL, got %v", args)
	}
	// The branch flag stays before the separator.
	if slices.Index(args, "-b") > sep {
		t.Errorf("branch flag should precede --, got %v", args)
	}
}

func TestCloneArgs_NoBranchOmitsFlag(t *testing.T) {
	args := cloneArgs("https://example.com/x.git", "/tmp/t", "")
	if slices.Contains(args, "-b") {
		t.Errorf("expected no -b flag, got %v", args)
	}
}
