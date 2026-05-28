package repo

import (
	"slices"
	"testing"

	"github.com/mad01/ralph/internal/config"
)

func TestBuildCloneArgs_InsertsSeparatorBeforePositionals(t *testing.T) {
	args := buildCloneArgs(config.Repo{URL: "-oProxyCommand=evil", Branch: "main"}, "/tmp/t")
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
