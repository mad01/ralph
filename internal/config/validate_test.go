package config

import (
	"os"
	"path/filepath"
	"strings"
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

func TestIsValidShellIdentifier(t *testing.T) {
	valid := []string{"foo", "MY_VAR", "_private", "a1", "FOO_BAR_123", "_"}
	for _, name := range valid {
		if !isValidShellIdentifier(name) {
			t.Errorf("isValidShellIdentifier(%q) = false, want true", name)
		}
	}

	invalid := []string{"", "foo bar", "123abc", "rm;evil", "a-b", "hello world", "x$y", "a.b"}
	for _, name := range invalid {
		if isValidShellIdentifier(name) {
			t.Errorf("isValidShellIdentifier(%q) = true, want false", name)
		}
	}
}

func TestValidateConfig_InvalidAliasName(t *testing.T) {
	badNames := []string{"bad;name", "foo|bar", "a&b", "has space", "x\ty", "foo$bar"}
	for _, name := range badNames {
		cfg := &Config{
			DotfilesRepoPath: "~/.dotfiles",
			Shell: ShellConfig{
				Aliases: map[string]ShellAlias{name: {Command: "ls"}},
			},
		}
		err := ValidateConfig(cfg)
		if err == nil {
			t.Errorf("expected error for alias name %q", name)
		} else if !strings.Contains(err.Error(), "invalid characters") {
			t.Errorf("unexpected error for %q: %v", name, err)
		}
	}
}

func TestValidateConfig_DotAliasNames(t *testing.T) {
	cfg := &Config{
		DotfilesRepoPath: "~/.dotfiles",
		Shell: ShellConfig{
			Aliases: map[string]ShellAlias{
				"...":            {Command: "cd ../.."},
				"....":           {Command: "cd ../../.."},
				"docker-compose": {Command: "docker compose"},
				"g.st":           {Command: "git status"},
			},
		},
	}
	if err := ValidateConfig(cfg); err != nil {
		t.Errorf("dot/dash alias names should be valid, got: %v", err)
	}
}

func TestValidateConfig_InvalidFunctionName(t *testing.T) {
	badNames := []string{"foo bar", "fn;inject", "a|b"}
	for _, name := range badNames {
		cfg := &Config{
			DotfilesRepoPath: "~/.dotfiles",
			Shell: ShellConfig{
				Functions: map[string]ShellFunction{name: {Body: "echo hi"}},
			},
		}
		err := ValidateConfig(cfg)
		if err == nil {
			t.Errorf("expected error for function name %q", name)
		} else if !strings.Contains(err.Error(), "invalid characters") {
			t.Errorf("unexpected error for %q: %v", name, err)
		}
	}
}

func TestValidateConfig_DashFunctionName(t *testing.T) {
	cfg := &Config{
		DotfilesRepoPath: "~/.dotfiles",
		Shell: ShellConfig{
			Functions: map[string]ShellFunction{
				"apply-system-kitty-theme": {Body: "echo theme"},
				"my.func":                  {Body: "echo dot"},
			},
		},
	}
	if err := ValidateConfig(cfg); err != nil {
		t.Errorf("dash/dot function names should be valid, got: %v", err)
	}
}

func TestValidateConfig_InvalidEnvVarName(t *testing.T) {
	cfg := &Config{
		DotfilesRepoPath: "~/.dotfiles",
		Shell: ShellConfig{
			Env: map[string]string{"123BAD": "value"},
		},
	}
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for invalid env var name")
	} else if !strings.Contains(err.Error(), "valid shell identifier") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateConfig_ValidShellIdentifiers(t *testing.T) {
	cfg := &Config{
		DotfilesRepoPath: "~/.dotfiles",
		Shell: ShellConfig{
			Aliases:   map[string]ShellAlias{"ll": {Command: "ls -alh"}, "_hidden": {Command: "ls"}, "....": {Command: "cd ../../.."}},
			Functions: map[string]ShellFunction{"my_func": {Body: "echo hi"}, "F1": {Body: "echo"}, "apply-theme": {Body: "echo theme"}},
			Env:       map[string]string{"HOME_DIR": "/home", "_x": "val"},
		},
	}
	if err := ValidateConfig(cfg); err != nil {
		t.Errorf("valid shell identifiers returned error: %v", err)
	}
}

func TestValidateConfig_BuildNegativeTimeout(t *testing.T) {
	cfg := &Config{
		DotfilesRepoPath: "~/.dotfiles",
		Hooks: HooksConfig{
			Builds: map[string]Build{
				"bad_build": {
					Commands: []string{"echo test"},
					Run:      "always",
					Timeout:  -1,
				},
			},
		},
	}
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for build with negative timeout")
	} else if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateConfig_PackageNegativeTimeout(t *testing.T) {
	cfg := &Config{
		DotfilesRepoPath: "~/.dotfiles",
		Packages: map[string]Package{
			"bad_pkg": {
				Source:   "local",
				WorkingDir: "/tmp",
				Build:    []string{"make"},
				Timeout:  -1,
			},
		},
	}
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for package with negative timeout")
	} else if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateConfig_BuildZeroTimeout(t *testing.T) {
	cfg := &Config{
		DotfilesRepoPath: "~/.dotfiles",
		Hooks: HooksConfig{
			Builds: map[string]Build{
				"ok_build": {
					Commands: []string{"echo test"},
					Run:      "always",
					Timeout:  0,
				},
			},
		},
	}
	if err := ValidateConfig(cfg); err != nil {
		t.Errorf("build with timeout=0 (default) should be valid, got: %v", err)
	}
}

func TestValidateConfig_PackagePositiveTimeout(t *testing.T) {
	cfg := &Config{
		DotfilesRepoPath: "~/.dotfiles",
		Packages: map[string]Package{
			"ok_pkg": {
				Source:     "local",
				WorkingDir: "/tmp",
				Build:      []string{"make"},
				Timeout:    5,
			},
		},
	}
	if err := ValidateConfig(cfg); err != nil {
		t.Errorf("package with timeout=5 should be valid, got: %v", err)
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
