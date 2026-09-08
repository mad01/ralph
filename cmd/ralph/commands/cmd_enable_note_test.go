package commands

import (
	"strings"
	"testing"

	"github.com/mad01/ralph/internal/config"
)

func TestSourceOverrideNote(t *testing.T) {
	cfg := &config.Config{
		SourceOverrideOrigins: map[string]string{"thismoon/catalog": "work"},
	}

	if got := sourceOverrideNote(cfg, "thismoon/reminder"); got != "" {
		t.Errorf("note for an un-overridden recipe = %q, want empty", got)
	}

	got := sourceOverrideNote(cfg, "thismoon/catalog")
	for _, want := range []string{
		"'work'",
		"thismoon/catalog",
		config.SourceOverridesFileName,
		"config.local.toml",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("note %q does not mention %s", got, want)
		}
	}
}
