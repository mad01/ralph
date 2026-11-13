package apply

import (
	"path/filepath"
	"time"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/state"
)

// IntendedManifest returns the recipe-state manifest a successful apply of
// cfg on host would record. It walks the same phase list Run does, calling
// each phase's declare step. Only items that came from a recipe (OwnerRecipe
// set) are recorded: an item defined straight in config.toml has no recipe
// to remove it when it goes away.
//
// Items that do not apply on this host (disabled, or filtered by host or
// profile) are not recorded, so their previously applied artifacts become
// orphans and are cleaned up. A recipe-level host or profile mismatch is
// different: cleanup carries those recipes forward frozen (see
// cleanup.KeepFrozenRecipes), because they belong to other machines.
//
// The second result is the set of filesystem paths declared by applying items
// that have no owner recipe. They are never recorded in state, but cleanup
// must protect them (see cleanup.ProtectedPaths).
//
// An error means an intended artifact set could not be enumerated (a
// dirs_mirror source a recipe owns is unreadable). Callers must not diff or
// save an incomplete manifest: live artifacts would look like orphans.
func IntendedManifest(
	cfg *config.Config,
	host string,
	now time.Time,
) (*state.RecipeState, map[string]bool, error) {
	m := newManifest(cfg, host, now)
	for _, p := range phases {
		if p.declare == nil {
			continue
		}
		if err := p.declare(m); err != nil {
			return nil, nil, err
		}
	}
	return m.state, m.unowned, nil
}

// manifest is the intended recipe-state manifest the declare steps fill.
type manifest struct {
	cfg     *config.Config
	host    string
	state   *state.RecipeState
	unowned map[string]bool // paths of applying items with no owner recipe
}

// newManifest returns an empty manifest for cfg with every loaded recipe's
// metadata in place, so delete behaviour, source and uninstall hooks are kept
// even for a recipe whose artifact set is empty this run.
func newManifest(cfg *config.Config, host string, now time.Time) *manifest {
	m := &manifest{
		cfg:  cfg,
		host: host,
		state: &state.RecipeState{
			Version: state.CurrentRecipeStateVersion,
			Recipes: map[string]state.RecipeArtifacts{},
		},
		unowned: map[string]bool{},
	}
	for _, info := range cfg.LoadedRecipes {
		m.state.SetMetadata(info.Name, now, string(info.DeleteBehavior.OrDelete()))
		m.state.SetRecipeSource(info.Name, info.RecipeSource)
		// Persist uninstall hooks so cleanup can run them when this recipe
		// later becomes an orphan, even if its recipe.toml is gone by then.
		m.state.SetUninstallHooks(info.Name, info.PreUninstall, info.PostUninstall)
	}
	return m
}

// applies reports whether an item with these filter fields applies on this
// host and profiles.
func (m *manifest) applies(enable *bool, hosts, profiles []string) bool {
	return config.Applies(enable, hosts, profiles, m.host, m.cfg.Profiles)
}

// track records a filesystem artifact under its owner recipe, or in unowned
// when the item has none.
func (m *manifest) track(owner string, kind state.ArtifactKind, path string) {
	if owner == "" {
		m.unowned[filepath.Clean(path)] = true
		return
	}
	m.state.AddArtifact(owner, kind, path)
}

// own records an artifact that is not a protected filesystem path (a repo, a
// shell alias or function, a package or build name, a package clone) under
// its owner recipe. An ownerless item records nothing.
func (m *manifest) own(owner string, kind state.ArtifactKind, value string) {
	m.state.AddArtifact(owner, kind, value)
}

// trackInstallPaths tracks each install path a build or package declares,
// skipping one that does not expand.
func (m *manifest) trackInstallPaths(owner string, paths []string) {
	for _, ip := range paths {
		expanded, err := config.ExpandPath(ip)
		if err != nil {
			continue
		}
		m.track(owner, state.KindInstallPath, expanded)
	}
}
