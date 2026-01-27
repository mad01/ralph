package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandPath(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get user home directory: %v", err)
	}

	tests := []struct {
		name     string
		input    string
		want     string
		wantErr  bool
		setupEnv map[string]string // For setting environment variables
	}{
		{
			name:  "tilde expansion",
			input: "~/testpath",
			want:  filepath.Join(homeDir, "testpath"),
		},
		{
			name:  "no tilde, no env vars",
			input: "/some/absolute/path",
			want:  "/some/absolute/path",
		},
		{
			name:     "with env var",
			input:    "$TEST_VAR/path",
			want:     "/tmp/testvalue/path",
			setupEnv: map[string]string{"TEST_VAR": "/tmp/testvalue"},
		},
		{
			name:     "tilde and env var",
			input:    "~/$TEST_VAR_SUFFIX",
			want:     filepath.Join(homeDir, "suffixpath"),
			setupEnv: map[string]string{"TEST_VAR_SUFFIX": "suffixpath"},
		},
		{
			name:  "empty path",
			input: "",
			want:  "",
		},
		{
			name:  "only tilde",
			input: "~",
			want:  homeDir,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variables for the test
			originalEnv := make(map[string]string)
			for key, value := range tt.setupEnv {
				if origVal, isset := os.LookupEnv(key); isset {
					originalEnv[key] = origVal
				}
				os.Setenv(key, value)
			}
			// Teardown: Restore original environment variables
			defer func() {
				for key := range tt.setupEnv {
					if origVal, isset := originalEnv[key]; isset {
						os.Setenv(key, origVal)
					} else {
						os.Unsetenv(key)
					}
				}
			}()

			got, err := ExpandPath(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExpandPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ExpandPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateConfig_Valid(t *testing.T) {
	cfg := &Config{
		DotfilesRepoPath: "~/.dotfiles",
		Dotfiles: map[string]Dotfile{
			"bashrc": {Source: ".bashrc", Target: "~/.bashrc"},
		},
		Tools: []Tool{
			{Name: "fzf", CheckCommand: "command -v fzf"},
		},
		Shell: ShellConfig{
			Aliases:   map[string]ShellAlias{"ll": {Command: "ls -alh"}},
			Functions: map[string]ShellFunction{"myfunc": {Body: "echo hello"}},
		},
	}
	if err := ValidateConfig(cfg); err != nil {
		t.Errorf("ValidateConfig() with valid config returned error: %v", err)
	}
}

func TestValidateConfig_MissingDotfilesRepoPath(t *testing.T) {
	cfg := &Config{}
	if err := ValidateConfig(cfg); err == nil {
		t.Error("ValidateConfig() with missing DotfilesRepoPath did not return an error")
	} else {
		t.Logf("Got expected error: %v", err) // Log error for visibility in test output
	}
}

func TestValidateConfig_DotfileMissingSource(t *testing.T) {
	cfg := &Config{
		DotfilesRepoPath: "~/.dotfiles",
		Dotfiles:         map[string]Dotfile{"missing_source": {Target: "~/.target"}},
	}
	if err := ValidateConfig(cfg); err == nil {
		t.Error("ValidateConfig() with dotfile missing source did not return an error")
	} else {
		t.Logf("Got expected error: %v", err)
	}
}

func TestValidateConfig_DotfileMissingTarget(t *testing.T) {
	cfg := &Config{
		DotfilesRepoPath: "~/.dotfiles",
		Dotfiles:         map[string]Dotfile{"missing_target": {Source: ".source"}},
	}
	if err := ValidateConfig(cfg); err == nil {
		t.Error("ValidateConfig() with dotfile missing target did not return an error")
	} else {
		t.Logf("Got expected error: %v", err)
	}
}

func TestValidateConfig_ToolMissingName(t *testing.T) {
	cfg := &Config{
		DotfilesRepoPath: "~/.dotfiles",
		Tools:            []Tool{{CheckCommand: "command -v tool"}},
	}
	if err := ValidateConfig(cfg); err == nil {
		t.Error("ValidateConfig() with tool missing name did not return an error")
	} else {
		t.Logf("Got expected error: %v", err)
	}
}

func TestValidateConfig_ToolMissingCheckCommand(t *testing.T) {
	cfg := &Config{
		DotfilesRepoPath: "~/.dotfiles",
		Tools:            []Tool{{Name: "mytool"}},
	}
	if err := ValidateConfig(cfg); err == nil {
		t.Error("ValidateConfig() with tool missing check_command did not return an error")
	} else {
		t.Logf("Got expected error: %v", err)
	}
}

func TestValidateConfig_ShellFunctionMissingBody(t *testing.T) {
	cfg := &Config{
		DotfilesRepoPath: "~/.dotfiles",
		Shell: ShellConfig{
			Functions: map[string]ShellFunction{"badfunc": {Body: ""}},
		},
	}
	if err := ValidateConfig(cfg); err == nil {
		t.Error("ValidateConfig() with shell function missing body did not return an error")
	} else {
		t.Logf("Got expected error: %v", err)
	}
}
