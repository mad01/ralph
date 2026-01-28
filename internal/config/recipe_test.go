package config

import (
	"os"
	"path/filepath"
	"testing"
)

func createTempRecipeFile(t *testing.T, dir, content string) string {
	t.Helper()
	filePath := filepath.Join(dir, RecipeFileName)
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write temp recipe file: %v", err)
	}
	return filePath
}

func TestLoadRecipe_Valid(t *testing.T) {
	tempDir := t.TempDir()
	content := `
[recipe]
name = "test-recipe"
description = "A test recipe"

[dotfiles.test_file]
source = "test.txt"
target = "~/.test.txt"

[shell.aliases.test]
command = "echo test"
`
	recipePath := createTempRecipeFile(t, tempDir, content)

	recipe, err := LoadRecipe(recipePath)
	if err != nil {
		t.Fatalf("LoadRecipe() returned error: %v", err)
	}

	if recipe.Recipe.Name != "test-recipe" {
		t.Errorf("Recipe name = %q, want %q", recipe.Recipe.Name, "test-recipe")
	}
	if recipe.Recipe.Description != "A test recipe" {
		t.Errorf("Recipe description = %q, want %q", recipe.Recipe.Description, "A test recipe")
	}
	if len(recipe.Dotfiles) != 1 {
		t.Errorf("len(Dotfiles) = %d, want 1", len(recipe.Dotfiles))
	}
	if df, ok := recipe.Dotfiles["test_file"]; !ok {
		t.Error("Dotfile 'test_file' not found")
	} else {
		if df.Source != "test.txt" {
			t.Errorf("Dotfile source = %q, want %q", df.Source, "test.txt")
		}
	}
}

func TestLoadRecipe_Invalid(t *testing.T) {
	tempDir := t.TempDir()
	content := `this is not valid toml{{{`
	recipePath := createTempRecipeFile(t, tempDir, content)

	_, err := LoadRecipe(recipePath)
	if err == nil {
		t.Error("LoadRecipe() with invalid TOML should return error")
	}
}

func TestLoadRecipe_NonExistent(t *testing.T) {
	_, err := LoadRecipe("/nonexistent/recipe.toml")
	if err == nil {
		t.Error("LoadRecipe() with non-existent file should return error")
	}
}

func TestResolveRecipePaths(t *testing.T) {
	recipe := &Recipe{
		Dotfiles: map[string]Dotfile{
			"file1": {Source: "config.txt", Target: "~/.config.txt"},
			"file2": {Source: "subdir/other.txt", Target: "~/.other.txt"},
		},
		Tools: []Tool{
			{
				Name:         "test-tool",
				CheckCommand: "which test",
				ConfigFiles: []Dotfile{
					{Source: "tool.conf", Target: "~/.tool.conf"},
				},
			},
		},
	}

	ResolveRecipePaths(recipe, "myrecipe")

	// Check dotfile paths
	if recipe.Dotfiles["file1"].Source != "myrecipe/config.txt" {
		t.Errorf("file1 source = %q, want %q", recipe.Dotfiles["file1"].Source, "myrecipe/config.txt")
	}
	if recipe.Dotfiles["file2"].Source != "myrecipe/subdir/other.txt" {
		t.Errorf("file2 source = %q, want %q", recipe.Dotfiles["file2"].Source, "myrecipe/subdir/other.txt")
	}

	// Check tool config file paths
	if recipe.Tools[0].ConfigFiles[0].Source != "myrecipe/tool.conf" {
		t.Errorf("tool config source = %q, want %q", recipe.Tools[0].ConfigFiles[0].Source, "myrecipe/tool.conf")
	}
}

func TestResolveRecipePaths_AbsolutePaths(t *testing.T) {
	recipe := &Recipe{
		Dotfiles: map[string]Dotfile{
			"file1": {Source: "/absolute/path/config.txt", Target: "~/.config.txt"},
		},
	}

	ResolveRecipePaths(recipe, "myrecipe")

	// Absolute paths should not be modified
	if recipe.Dotfiles["file1"].Source != "/absolute/path/config.txt" {
		t.Errorf("Absolute path was modified: %q", recipe.Dotfiles["file1"].Source)
	}
}

func TestMergeRecipeIntoConfig_Success(t *testing.T) {
	cfg := &Config{
		DotfilesRepoPath: "~/.dotfiles",
		Dotfiles: map[string]Dotfile{
			"existing": {Source: "existing.txt", Target: "~/.existing"},
		},
		Shell: ShellConfig{
			Aliases: map[string]ShellAlias{
				"existing_alias": {Command: "echo existing"},
			},
		},
	}

	recipe := &Recipe{
		Dotfiles: map[string]Dotfile{
			"new_file": {Source: "new.txt", Target: "~/.new"},
		},
		Shell: ShellConfig{
			Aliases: map[string]ShellAlias{
				"new_alias": {Command: "echo new"},
			},
		},
	}

	err := MergeRecipeIntoConfig(cfg, recipe, "test-recipe")
	if err != nil {
		t.Fatalf("MergeRecipeIntoConfig() returned error: %v", err)
	}

	// Check dotfiles merged
	if len(cfg.Dotfiles) != 2 {
		t.Errorf("len(Dotfiles) = %d, want 2", len(cfg.Dotfiles))
	}
	if _, ok := cfg.Dotfiles["new_file"]; !ok {
		t.Error("new_file not found in merged config")
	}

	// Check aliases merged
	if len(cfg.Shell.Aliases) != 2 {
		t.Errorf("len(Aliases) = %d, want 2", len(cfg.Shell.Aliases))
	}
	if _, ok := cfg.Shell.Aliases["new_alias"]; !ok {
		t.Error("new_alias not found in merged config")
	}
}

func TestMergeRecipeIntoConfig_Conflict(t *testing.T) {
	cfg := &Config{
		DotfilesRepoPath: "~/.dotfiles",
		Dotfiles: map[string]Dotfile{
			"conflict": {Source: "original.txt", Target: "~/.conflict"},
		},
	}

	recipe := &Recipe{
		Dotfiles: map[string]Dotfile{
			"conflict": {Source: "new.txt", Target: "~/.conflict"},
		},
	}

	err := MergeRecipeIntoConfig(cfg, recipe, "test-recipe")
	if err == nil {
		t.Error("MergeRecipeIntoConfig() should return error on conflict")
	}
}

func TestMergeRecipeIntoConfig_AllTypes(t *testing.T) {
	cfg := &Config{
		DotfilesRepoPath: "~/.dotfiles",
	}

	recipe := &Recipe{
		Dotfiles: map[string]Dotfile{
			"df": {Source: "file.txt", Target: "~/.file"},
		},
		Directories: map[string]Directory{
			"dir": {Target: "~/mydir"},
		},
		Repos: map[string]Repo{
			"repo": {URL: "https://github.com/test/repo.git", Target: "~/repo"},
		},
		Tools: []Tool{
			{Name: "tool", CheckCommand: "which tool"},
		},
		Shell: ShellConfig{
			Aliases:   map[string]ShellAlias{"alias": {Command: "echo"}},
			Functions: map[string]ShellFunction{"func": {Body: "echo"}},
			Env:       map[string]string{"VAR": "value"},
		},
		Hooks: HooksConfig{
			PreApply:  []string{"echo pre"},
			PostApply: []string{"echo post"},
			PreLink:   map[string][]string{"df": {"echo prelink"}},
			PostLink:  map[string][]string{"df": {"echo postlink"}},
			Builds:    map[string]Build{"build": {Commands: []string{"make"}, Run: "once"}},
		},
		TemplateVariables: map[string]interface{}{"var": "value"},
	}

	err := MergeRecipeIntoConfig(cfg, recipe, "test")
	if err != nil {
		t.Fatalf("MergeRecipeIntoConfig() returned error: %v", err)
	}

	// Verify all types were merged
	if len(cfg.Dotfiles) != 1 {
		t.Errorf("Dotfiles not merged correctly")
	}
	if len(cfg.Directories) != 1 {
		t.Errorf("Directories not merged correctly")
	}
	if len(cfg.Repos) != 1 {
		t.Errorf("Repos not merged correctly")
	}
	if len(cfg.Tools) != 1 {
		t.Errorf("Tools not merged correctly")
	}
	if len(cfg.Shell.Aliases) != 1 {
		t.Errorf("Aliases not merged correctly")
	}
	if len(cfg.Shell.Functions) != 1 {
		t.Errorf("Functions not merged correctly")
	}
	if len(cfg.Shell.Env) != 1 {
		t.Errorf("Env vars not merged correctly")
	}
	if len(cfg.Hooks.PreApply) != 1 {
		t.Errorf("PreApply hooks not merged correctly")
	}
	if len(cfg.Hooks.PostApply) != 1 {
		t.Errorf("PostApply hooks not merged correctly")
	}
	if len(cfg.Hooks.PreLink) != 1 {
		t.Errorf("PreLink hooks not merged correctly")
	}
	if len(cfg.Hooks.PostLink) != 1 {
		t.Errorf("PostLink hooks not merged correctly")
	}
	if len(cfg.Hooks.Builds) != 1 {
		t.Errorf("Builds not merged correctly")
	}
	if len(cfg.TemplateVariables) != 1 {
		t.Errorf("TemplateVariables not merged correctly")
	}
}

func TestDiscoverRecipes(t *testing.T) {
	// Create a temp directory structure with recipes in recipes/ subdirectory
	tempDir := t.TempDir()

	// Create recipe directories under recipes/
	os.MkdirAll(filepath.Join(tempDir, "recipes", "editors"), 0755)
	os.MkdirAll(filepath.Join(tempDir, "recipes", "shell"), 0755)
	os.MkdirAll(filepath.Join(tempDir, "recipes", "excluded"), 0755)

	// Create recipe files
	os.WriteFile(filepath.Join(tempDir, "recipes", "editors", "recipe.toml"), []byte(`[recipe]
name = "editors"
`), 0644)
	os.WriteFile(filepath.Join(tempDir, "recipes", "shell", "recipe.toml"), []byte(`[recipe]
name = "shell"
`), 0644)
	os.WriteFile(filepath.Join(tempDir, "recipes", "excluded", "recipe.toml"), []byte(`[recipe]
name = "excluded"
`), 0644)

	// Test without exclusions
	recipes, err := DiscoverRecipes(tempDir, RecipesConfig{})
	if err != nil {
		t.Fatalf("DiscoverRecipes() returned error: %v", err)
	}
	if len(recipes) != 3 {
		t.Errorf("len(recipes) = %d, want 3", len(recipes))
	}

	// Test with exclusion
	recipes, err = DiscoverRecipes(tempDir, RecipesConfig{
		Exclude: []string{"excluded/*"},
	})
	if err != nil {
		t.Fatalf("DiscoverRecipes() with exclusion returned error: %v", err)
	}
	if len(recipes) != 2 {
		t.Errorf("len(recipes) with exclusion = %d, want 2", len(recipes))
	}
}

func TestDiscoverRecipes_WithOverrides(t *testing.T) {
	tempDir := t.TempDir()
	os.MkdirAll(filepath.Join(tempDir, "recipes", "work"), 0755)
	os.WriteFile(filepath.Join(tempDir, "recipes", "work", "recipe.toml"), []byte(`[recipe]
name = "work"
`), 0644)

	falseVal := false
	recipes, err := DiscoverRecipes(tempDir, RecipesConfig{
		Overrides: map[string]RecipeOverride{
			"work": {
				Enable: &falseVal,
				Hosts:  []string{"work-laptop"},
			},
		},
	})
	if err != nil {
		t.Fatalf("DiscoverRecipes() returned error: %v", err)
	}
	if len(recipes) != 1 {
		t.Fatalf("len(recipes) = %d, want 1", len(recipes))
	}

	if recipes[0].Enable == nil || *recipes[0].Enable != false {
		t.Error("Recipe enable override not applied")
	}
	if len(recipes[0].Hosts) != 1 || recipes[0].Hosts[0] != "work-laptop" {
		t.Error("Recipe hosts override not applied")
	}
}

func TestProcessRecipes_Explicit(t *testing.T) {
	tempDir := t.TempDir()

	// Create recipe with explicit path
	recipeDir := filepath.Join(tempDir, "myrecipe")
	os.MkdirAll(recipeDir, 0755)
	os.WriteFile(filepath.Join(recipeDir, "recipe.toml"), []byte(`
[recipe]
name = "myrecipe"

[dotfiles.myfile]
source = "file.txt"
target = "~/.myfile"
`), 0644)

	cfg := &Config{
		DotfilesRepoPath: tempDir,
		Recipes: []RecipeRef{
			{Path: "myrecipe/recipe.toml"},
		},
	}

	err := ProcessRecipes(cfg, "test-host")
	if err != nil {
		t.Fatalf("ProcessRecipes() returned error: %v", err)
	}

	if len(cfg.Dotfiles) != 1 {
		t.Errorf("len(Dotfiles) = %d, want 1", len(cfg.Dotfiles))
	}

	// Check path was resolved
	if df, ok := cfg.Dotfiles["myfile"]; ok {
		if df.Source != "myrecipe/file.txt" {
			t.Errorf("Dotfile source = %q, want %q", df.Source, "myrecipe/file.txt")
		}
	} else {
		t.Error("Dotfile 'myfile' not found")
	}

	// Check loaded recipes info
	if len(cfg.LoadedRecipes) != 1 {
		t.Errorf("len(LoadedRecipes) = %d, want 1", len(cfg.LoadedRecipes))
	}
}

func TestProcessRecipes_ShortName(t *testing.T) {
	tempDir := t.TempDir()

	// Create recipe with short name (in recipes/<name>/recipe.toml)
	recipeDir := filepath.Join(tempDir, "recipes", "shell")
	os.MkdirAll(recipeDir, 0755)
	os.WriteFile(filepath.Join(recipeDir, "recipe.toml"), []byte(`
[recipe]
name = "shell"

[dotfiles.zshrc]
source = "zshrc"
target = "~/.zshrc"
`), 0644)

	cfg := &Config{
		DotfilesRepoPath: tempDir,
		Recipes: []RecipeRef{
			{Name: "shell"}, // Short name instead of path
		},
	}

	err := ProcessRecipes(cfg, "test-host")
	if err != nil {
		t.Fatalf("ProcessRecipes() returned error: %v", err)
	}

	if len(cfg.Dotfiles) != 1 {
		t.Errorf("len(Dotfiles) = %d, want 1", len(cfg.Dotfiles))
	}

	// Check path was resolved correctly
	if df, ok := cfg.Dotfiles["zshrc"]; ok {
		expected := "recipes/shell/zshrc"
		if df.Source != expected {
			t.Errorf("Dotfile source = %q, want %q", df.Source, expected)
		}
	} else {
		t.Error("Dotfile 'zshrc' not found")
	}
}

func TestResolveRecipeRefPath(t *testing.T) {
	tests := []struct {
		name       string
		ref        RecipeRef
		recipesDir string
		want       string
	}{
		{
			name:       "path takes precedence",
			ref:        RecipeRef{Path: "custom/path/recipe.toml", Name: "ignored"},
			recipesDir: "recipes",
			want:       "custom/path/recipe.toml",
		},
		{
			name:       "name with default dir",
			ref:        RecipeRef{Name: "shell"},
			recipesDir: "",
			want:       "recipes/shell/recipe.toml",
		},
		{
			name:       "name with custom dir",
			ref:        RecipeRef{Name: "editors"},
			recipesDir: "my-recipes",
			want:       "my-recipes/editors/recipe.toml",
		},
		{
			name:       "empty ref",
			ref:        RecipeRef{},
			recipesDir: "recipes",
			want:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveRecipeRefPath(tt.ref, tt.recipesDir)
			if got != tt.want {
				t.Errorf("ResolveRecipeRefPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProcessRecipes_AutoDiscover(t *testing.T) {
	tempDir := t.TempDir()

	// Create multiple recipes in recipes/ subdirectory
	for _, name := range []string{"recipe1", "recipe2"} {
		dir := filepath.Join(tempDir, "recipes", name)
		os.MkdirAll(dir, 0755)
		os.WriteFile(filepath.Join(dir, "recipe.toml"), []byte(`
[recipe]
name = "`+name+`"

[dotfiles.`+name+`_file]
source = "file.txt"
target = "~/.`+name+`"
`), 0644)
	}

	cfg := &Config{
		DotfilesRepoPath: tempDir,
		RecipesConfig: RecipesConfig{
			AutoDiscover: true,
		},
	}

	err := ProcessRecipes(cfg, "test-host")
	if err != nil {
		t.Fatalf("ProcessRecipes() returned error: %v", err)
	}

	if len(cfg.Dotfiles) != 2 {
		t.Errorf("len(Dotfiles) = %d, want 2", len(cfg.Dotfiles))
	}
}

func TestProcessRecipes_DisabledRecipe(t *testing.T) {
	tempDir := t.TempDir()

	// Create recipe
	recipeDir := filepath.Join(tempDir, "disabled")
	os.MkdirAll(recipeDir, 0755)
	os.WriteFile(filepath.Join(recipeDir, "recipe.toml"), []byte(`
[recipe]
name = "disabled"

[dotfiles.file]
source = "file.txt"
target = "~/.file"
`), 0644)

	falseVal := false
	cfg := &Config{
		DotfilesRepoPath: tempDir,
		Recipes: []RecipeRef{
			{Path: "disabled/recipe.toml", Enable: &falseVal},
		},
	}

	err := ProcessRecipes(cfg, "test-host")
	if err != nil {
		t.Fatalf("ProcessRecipes() returned error: %v", err)
	}

	if len(cfg.Dotfiles) != 0 {
		t.Errorf("Disabled recipe should not add dotfiles, got %d", len(cfg.Dotfiles))
	}
}

func TestProcessRecipes_HostFiltered(t *testing.T) {
	tempDir := t.TempDir()

	// Create recipe
	recipeDir := filepath.Join(tempDir, "workonly")
	os.MkdirAll(recipeDir, 0755)
	os.WriteFile(filepath.Join(recipeDir, "recipe.toml"), []byte(`
[recipe]
name = "workonly"

[dotfiles.file]
source = "file.txt"
target = "~/.file"
`), 0644)

	cfg := &Config{
		DotfilesRepoPath: tempDir,
		Recipes: []RecipeRef{
			{Path: "workonly/recipe.toml", Hosts: []string{"work-laptop"}},
		},
	}

	// Test with non-matching host
	err := ProcessRecipes(cfg, "home-desktop")
	if err != nil {
		t.Fatalf("ProcessRecipes() returned error: %v", err)
	}
	if len(cfg.Dotfiles) != 0 {
		t.Errorf("Host-filtered recipe should not add dotfiles on non-matching host")
	}

	// Reset and test with matching host
	cfg.Dotfiles = nil
	cfg.LoadedRecipes = nil
	err = ProcessRecipes(cfg, "work-laptop")
	if err != nil {
		t.Fatalf("ProcessRecipes() returned error: %v", err)
	}
	if len(cfg.Dotfiles) != 1 {
		t.Errorf("Host-filtered recipe should add dotfiles on matching host")
	}
}

func TestProcessRecipes_RecipeHostFilterInheritance(t *testing.T) {
	tempDir := t.TempDir()

	// Create recipe with items that don't have host filters
	recipeDir := filepath.Join(tempDir, "work")
	os.MkdirAll(recipeDir, 0755)
	os.WriteFile(filepath.Join(recipeDir, "recipe.toml"), []byte(`
[recipe]
name = "work"

[dotfiles.file1]
source = "file1.txt"
target = "~/.file1"

[dotfiles.file2]
source = "file2.txt"
target = "~/.file2"
hosts = ["specific-host"]
`), 0644)

	cfg := &Config{
		DotfilesRepoPath: tempDir,
		Recipes: []RecipeRef{
			{Path: "work/recipe.toml", Hosts: []string{"work-laptop"}},
		},
	}

	err := ProcessRecipes(cfg, "work-laptop")
	if err != nil {
		t.Fatalf("ProcessRecipes() returned error: %v", err)
	}

	// file1 should inherit recipe host filter
	if df, ok := cfg.Dotfiles["file1"]; ok {
		if len(df.Hosts) != 1 || df.Hosts[0] != "work-laptop" {
			t.Errorf("file1 should inherit recipe hosts, got %v", df.Hosts)
		}
	}

	// file2 should keep its own host filter
	if df, ok := cfg.Dotfiles["file2"]; ok {
		if len(df.Hosts) != 1 || df.Hosts[0] != "specific-host" {
			t.Errorf("file2 should keep its own hosts, got %v", df.Hosts)
		}
	}
}

func TestProcessRecipes_LegacyPaths(t *testing.T) {
	tempDir := t.TempDir()

	// Create recipe with legacy paths
	recipeDir := filepath.Join(tempDir, "editors")
	os.MkdirAll(recipeDir, 0755)
	os.WriteFile(filepath.Join(recipeDir, "recipe.toml"), []byte(`
[recipe]
name = "editors"

[recipe.legacy_paths]
"dotter_files/nvim/init.lua" = "nvim/init.lua"
"dotter_files/nvim" = "nvim"

[dotfiles.nvim_init]
source = "nvim/init.lua"
target = "~/.config/nvim/init.lua"
`), 0644)

	cfg := &Config{
		DotfilesRepoPath: tempDir,
		Recipes: []RecipeRef{
			{Path: "editors/recipe.toml"},
		},
	}

	err := ProcessRecipes(cfg, "test-host")
	if err != nil {
		t.Fatalf("ProcessRecipes() returned error: %v", err)
	}

	// Check legacy paths are stored
	if len(cfg.LoadedRecipes) != 1 {
		t.Fatalf("Expected 1 loaded recipe, got %d", len(cfg.LoadedRecipes))
	}
	if len(cfg.LoadedRecipes[0].LegacyPaths) != 2 {
		t.Errorf("Expected 2 legacy paths, got %d", len(cfg.LoadedRecipes[0].LegacyPaths))
	}
}

func TestGetAllLegacyPaths(t *testing.T) {
	cfg := &Config{
		LoadedRecipes: []LoadedRecipeInfo{
			{
				Path: "editors/recipe.toml",
				Dir:  "editors",
				LegacyPaths: map[string]string{
					"dotter_files/nvim": "nvim",
				},
			},
			{
				Path: "shell/recipe.toml",
				Dir:  "shell",
				LegacyPaths: map[string]string{
					"dotter_files/zshrc": "zshrc",
				},
			},
		},
	}

	legacyPaths := GetAllLegacyPaths(cfg)

	if len(legacyPaths) != 2 {
		t.Errorf("Expected 2 legacy paths, got %d", len(legacyPaths))
	}

	// Check paths are resolved relative to recipe dir
	if newPath, ok := legacyPaths["dotter_files/nvim"]; ok {
		if newPath != "editors/nvim" {
			t.Errorf("Legacy path not resolved correctly: got %q, want %q", newPath, "editors/nvim")
		}
	} else {
		t.Error("Legacy path 'dotter_files/nvim' not found")
	}
}

func TestLoadConfig_WithRecipes(t *testing.T) {
	// Create temp directory structure
	tempDir := t.TempDir()
	dotfilesDir := filepath.Join(tempDir, "dotfiles")
	os.MkdirAll(dotfilesDir, 0755)

	// Create recipe
	recipeDir := filepath.Join(dotfilesDir, "test")
	os.MkdirAll(recipeDir, 0755)
	os.WriteFile(filepath.Join(recipeDir, "recipe.toml"), []byte(`
[recipe]
name = "test"

[dotfiles.testfile]
source = "file.txt"
target = "~/.testfile"
`), 0644)

	// Create main config
	configDir := filepath.Join(tempDir, "config")
	os.MkdirAll(configDir, 0755)
	configContent := `
dotfiles_repo_path = "` + dotfilesDir + `"

[[recipes]]
path = "test/recipe.toml"
`
	os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(configContent), 0644)

	// Override config path
	originalGetDefaultConfigPath := GetDefaultConfigPath
	GetDefaultConfigPath = func() (string, error) {
		return filepath.Join(configDir, "config.toml"), nil
	}
	defer func() { GetDefaultConfigPath = originalGetDefaultConfigPath }()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() returned error: %v", err)
	}

	if len(cfg.Dotfiles) != 1 {
		t.Errorf("Expected 1 dotfile from recipe, got %d", len(cfg.Dotfiles))
	}
}

func TestLoadConfig_BackwardCompatible(t *testing.T) {
	// Test that configs without recipes still work
	tempDir := t.TempDir()
	dotfilesDir := filepath.Join(tempDir, "dotfiles")
	os.MkdirAll(dotfilesDir, 0755)

	configDir := filepath.Join(tempDir, "config")
	os.MkdirAll(configDir, 0755)
	configContent := `
dotfiles_repo_path = "` + dotfilesDir + `"

[dotfiles.bashrc]
source = ".bashrc"
target = "~/.bashrc"
`
	os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(configContent), 0644)

	originalGetDefaultConfigPath := GetDefaultConfigPath
	GetDefaultConfigPath = func() (string, error) {
		return filepath.Join(configDir, "config.toml"), nil
	}
	defer func() { GetDefaultConfigPath = originalGetDefaultConfigPath }()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() returned error: %v", err)
	}

	if len(cfg.Dotfiles) != 1 {
		t.Errorf("Expected 1 dotfile, got %d", len(cfg.Dotfiles))
	}
	if _, ok := cfg.Dotfiles["bashrc"]; !ok {
		t.Error("Dotfile 'bashrc' not found")
	}
}
