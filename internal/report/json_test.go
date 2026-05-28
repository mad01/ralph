package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

// sampleReport builds a report exercising every status plus recipe ownership.
func sampleReport() *Report {
	r := &Report{Command: "up"}
	p := r.AddPhase("Dotfiles")
	p.AddOK("bashrc", "linked")
	p.AddResult("broken", "web", StatusFail, "could not link", errors.New("boom"))
	p.AddWarn("vimrc", "already exists")
	p.AddSkip("zshrc", "host filtered")
	return r
}

func TestToJSONCountsAndStatuses(t *testing.T) {
	jr := sampleReport().ToJSON(false)

	if jr.Command != "up" {
		t.Errorf("Command = %q, want up", jr.Command)
	}
	if jr.DryRun {
		t.Error("DryRun = true, want false")
	}
	if jr.Summary.OK != 1 || jr.Summary.Warnings != 1 || jr.Summary.Failed != 1 || jr.Summary.Skipped != 1 {
		t.Errorf("Summary = %+v, want 1/1/1/1", jr.Summary)
	}
	if jr.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1 (has failures)", jr.ExitCode)
	}
	if len(jr.Phases) != 1 || jr.Phases[0].Name != "Dotfiles" {
		t.Fatalf("Phases = %+v, want one Dotfiles phase", jr.Phases)
	}

	wantStatus := map[string]string{
		"bashrc": "ok",
		"broken": "fail",
		"vimrc":  "warn",
		"zshrc":  "skip",
	}
	for _, s := range jr.Phases[0].Steps {
		if want := wantStatus[s.Name]; s.Status != want {
			t.Errorf("step %s status = %q, want %q", s.Name, s.Status, want)
		}
	}
}

func TestToJSONErrorAndRecipeFields(t *testing.T) {
	jr := sampleReport().ToJSON(false)
	var broken, bashrc *JSONStep
	for i := range jr.Phases[0].Steps {
		switch jr.Phases[0].Steps[i].Name {
		case "broken":
			broken = &jr.Phases[0].Steps[i]
		case "bashrc":
			bashrc = &jr.Phases[0].Steps[i]
		}
	}
	if broken == nil || bashrc == nil {
		t.Fatal("expected broken and bashrc steps")
	}
	if broken.Error != "boom" {
		t.Errorf("broken.Error = %q, want boom", broken.Error)
	}
	if broken.Recipe != "web" {
		t.Errorf("broken.Recipe = %q, want web", broken.Recipe)
	}
	if bashrc.Error != "" {
		t.Errorf("bashrc.Error = %q, want empty (no error)", bashrc.Error)
	}
}

func TestToJSONDryRun(t *testing.T) {
	if !(&Report{Command: "up"}).ToJSON(true).DryRun {
		t.Error("ToJSON(true).DryRun = false, want true")
	}
}

func TestWriteJSONIsValidAndParsesBack(t *testing.T) {
	var buf bytes.Buffer
	if err := sampleReport().WriteJSON(&buf, false); err != nil {
		t.Fatalf("WriteJSON error: %v", err)
	}
	out := buf.Bytes()
	if len(out) == 0 || out[len(out)-1] != '\n' {
		t.Error("WriteJSON output should end with a trailing newline")
	}

	// Must round-trip into the same structured shape an integration test would
	// parse via jq — assert on the exact snake_case keys we emit.
	var parsed struct {
		Command string `json:"command"`
		DryRun  bool   `json:"dry_run"`
		Summary struct {
			OK       int `json:"ok"`
			Warnings int `json:"warnings"`
			Failed   int `json:"failed"`
			Skipped  int `json:"skipped"`
		} `json:"summary"`
		ExitCode int `json:"exit_code"`
		Phases   []struct {
			Name  string `json:"name"`
			Steps []struct {
				Name    string `json:"name"`
				Status  string `json:"status"`
				Message string `json:"message"`
				Recipe  string `json:"recipe"`
				Error   string `json:"error"`
			} `json:"steps"`
		} `json:"phases"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if parsed.Command != "up" || parsed.ExitCode != 1 {
		t.Errorf("parsed Command=%q ExitCode=%d, want up/1", parsed.Command, parsed.ExitCode)
	}
	if parsed.Summary.Failed != 1 {
		t.Errorf("parsed Summary.Failed = %d, want 1", parsed.Summary.Failed)
	}
	if len(parsed.Phases) != 1 || len(parsed.Phases[0].Steps) != 4 {
		t.Fatalf("parsed phases/steps mismatch: %+v", parsed.Phases)
	}
}
