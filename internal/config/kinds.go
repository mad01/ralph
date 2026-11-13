package config

// LinkAction is how a dotfile or a dirs_mirror entry reaches its target.
// Empty means ActionSymlink.
type LinkAction string

const (
	// ActionSymlink links the target to the source file. It is the default.
	ActionSymlink LinkAction = "symlink"
	// ActionCopy copies the source to the target. Dotfiles only.
	ActionCopy LinkAction = "copy"
	// ActionSymlinkDir links the target to the source directory as a whole.
	ActionSymlinkDir LinkAction = "symlink_dir"
)

// RunMode is when a build runs.
type RunMode string

const (
	// RunAlways runs the build on every apply.
	RunAlways RunMode = "always"
	// RunOnce runs the build until it has succeeded once, then again only
	// when its working_dir subtree changes.
	RunOnce RunMode = "once"
	// RunManual runs the build only when it is named with --build.
	RunManual RunMode = "manual"
)

// PackageSource is where a package's code comes from. Empty means
// SourceLocal.
type PackageSource string

const (
	// SourceLocal builds in a working_dir that ralph does not clone.
	SourceLocal PackageSource = "local"
	// SourceRemote clones a git repo and builds in it.
	SourceRemote PackageSource = "remote"
	// SourceMake is SourceRemote with "make build" and "make install" as the
	// default commands.
	SourceMake PackageSource = "make"
	// SourceGoInstall runs go install module@version.
	SourceGoInstall PackageSource = "go-install"
)

// OrLocal returns s, or SourceLocal when s is empty.
func (s PackageSource) OrLocal() PackageSource {
	if s == "" {
		return SourceLocal
	}
	return s
}

// Clones reports whether ralph clones the package from a git remote.
func (s PackageSource) Clones() bool {
	return s == SourceRemote || s == SourceMake
}

// DeleteBehavior is what cleanup does with a recipe's orphaned artifacts.
// Empty means DeleteBehaviorDelete.
type DeleteBehavior string

const (
	// DeleteBehaviorDelete removes orphaned artifacts when a recipe is gone.
	DeleteBehaviorDelete DeleteBehavior = "delete"
	// DeleteBehaviorAbandon leaves orphaned artifacts in place when a recipe
	// is gone.
	DeleteBehaviorAbandon DeleteBehavior = "abandon"
)

// OrDelete returns b, or DeleteBehaviorDelete when b is empty.
func (b DeleteBehavior) OrDelete() DeleteBehavior {
	if b == "" {
		return DeleteBehaviorDelete
	}
	return b
}
