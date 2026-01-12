package taskfile

import (
	"runtime"
	"strings"
)

// Archetype represents the type of Taskfile based on its purpose.
type Archetype string

const (
	// ArchetypeTool - Binary we build & release (has _BIN + _VERSION + _REPO)
	ArchetypeTool Archetype = "tool"

	// ArchetypeExternal - External binary we install (has _BIN + _VERSION, no _REPO)
	ArchetypeExternal Archetype = "external"

	// ArchetypeBuilder - Provides build: task for Tools (has build: task, no _BIN)
	ArchetypeBuilder Archetype = "builder"

	// ArchetypeAggregation - Parent with includes: section
	ArchetypeAggregation Archetype = "aggregation"

	// ArchetypeBootstrap - Self-bootstrapping tool (xplat itself)
	ArchetypeBootstrap Archetype = "bootstrap"

	// ArchetypeUnknown - Could not detect archetype
	ArchetypeUnknown Archetype = "unknown"
)

// ArchetypeInfo provides details about a detected archetype.
type ArchetypeInfo struct {
	Type          Archetype
	Description   string
	RequiredVars  []string
	RequiredTasks []string
}

// DetectArchetype determines the archetype of a Taskfile based on its structure.
//
// Detection logic (in order):
// 0. Explicit XPLAT_ARCHETYPE marker (e.g., XPLAT_ARCHETYPE: builder) -> Use that value
// 1. Has includes: section -> Aggregation
// 2. Has _BUILD_* vars (indicates build infrastructure provider) -> Builder
// 3. Has _BIN + _VERSION + _REPO vars -> Tool (we build it)
// 4. Has _BIN + _VERSION but no _REPO -> External (we install it)
// 5. Otherwise -> Unknown (project workflow, no strict requirements)
func DetectArchetype(tf *Taskfile) ArchetypeInfo {
	// 0. Check for explicit XPLAT_ARCHETYPE marker (highest priority)
	// This allows manual override and makes intent clear
	// Single consistent var name across all taskfiles
	archetype, ok := tf.Vars["XPLAT_ARCHETYPE"].(string)
	if ok && archetype != "" {
		switch archetype {
		case "tool":
			return ArchetypeInfo{
				Type:          ArchetypeTool,
				Description:   "Tool taskfile for a binary we build and release",
				RequiredVars:  []string{"_VERSION", "_REPO", "_BIN", "_CGO"},
				RequiredTasks: []string{"check:deps", "release:build", "release:test"},
			}
		case "external":
			return ArchetypeInfo{
				Type:          ArchetypeExternal,
				Description:   "External tool taskfile for a binary we install",
				RequiredVars:  []string{"_VERSION", "_BIN"},
				RequiredTasks: []string{"check:deps"},
			}
		case "builder":
			return ArchetypeInfo{
				Type:          ArchetypeBuilder,
				Description:   "Builder taskfile that provides build infrastructure",
				RequiredTasks: []string{"check:deps", "build"},
			}
		case "bootstrap":
			return ArchetypeInfo{
				Type:          ArchetypeBootstrap,
				Description:   "Bootstrap taskfile (self-installing, no external deps)",
				RequiredVars:  []string{"_VERSION", "_REPO", "_BIN"},
				RequiredTasks: []string{"check:deps"},
			}
		}
	}

	// 1. Check for Aggregation (has includes:)
	if len(tf.Includes) > 0 {
		return ArchetypeInfo{
			Type:        ArchetypeAggregation,
			Description: "Aggregation taskfile that includes children",
			// No required vars - delegates to children
		}
	}

	// 2. Check for Bootstrap (xplat itself - self-bootstrapping)
	// Bootstrap is detected by having XPLAT_* vars AND being the xplat taskfile
	// It's special because it can't depend on xplat binary:install (chicken-egg)
	if tf.HasVar("XPLAT_VERSION") && tf.HasVar("XPLAT_REPO") && tf.HasVar("XPLAT_BIN") {
		return ArchetypeInfo{
			Type:          ArchetypeBootstrap,
			Description:   "Bootstrap taskfile (self-installing, no external deps)",
			RequiredVars:  []string{"_VERSION", "_REPO", "_BIN"},
			RequiredTasks: []string{"check:deps"},
			// No release:build/test required - bootstrap builds itself differently
		}
	}

	// 3. Check for Builder (has *_BUILD_* vars - build infrastructure provider)
	// Examples: GO_BUILD_DIR, RUST_BUILD_PLATFORMS, BUN_BUILD_LDFLAGS
	if tf.HasVar("_BUILD_") {
		return ArchetypeInfo{
			Type:          ArchetypeBuilder,
			Description:   "Builder taskfile that provides build infrastructure",
			RequiredTasks: []string{"check:deps", "build"},
		}
	}

	// Check for binary vars
	hasBin := tf.HasVar("_BIN")
	hasVersion := tf.HasVar("_VERSION")
	hasRepo := tf.HasVar("_REPO")

	// 4. Check for Tool (has _BIN + _VERSION + _REPO)
	if hasBin && hasVersion && hasRepo {
		return ArchetypeInfo{
			Type:          ArchetypeTool,
			Description:   "Tool taskfile for a binary we build and release",
			RequiredVars:  []string{"_VERSION", "_REPO", "_BIN", "_CGO"},
			RequiredTasks: []string{"check:deps", "release:build", "release:test"},
		}
	}

	// 5. Check for External (has _BIN + _VERSION, no _REPO)
	if hasBin && hasVersion && !hasRepo {
		return ArchetypeInfo{
			Type:          ArchetypeExternal,
			Description:   "External tool taskfile for a binary we install",
			RequiredVars:  []string{"_VERSION", "_BIN"},
			RequiredTasks: []string{"check:deps"},
		}
	}

	// 6. Unknown - project workflow taskfile with no strict requirements
	// Examples: hugo.yml, translate.yml - they have their own lifecycle
	return ArchetypeInfo{
		Type:        ArchetypeUnknown,
		Description: "Project workflow taskfile",
	}
}

// String returns the archetype name.
func (a Archetype) String() string {
	return string(a)
}

// Affinity represents whether a tool can cross-compile or must build natively.
type Affinity string

const (
	// AffinityCross means tool can cross-compile from any platform (CGO=0)
	AffinityCross Affinity = "cross"
	// AffinityNative means tool must build on native platform (CGO=1)
	AffinityNative Affinity = "native"
)

// String returns the affinity name.
func (a Affinity) String() string {
	return string(a)
}

// DetectAffinity determines cross-compile capability from Taskfile.
// This is runtime-agnostic - works for Go (CGO), Rust (native deps), etc.
func DetectAffinity(tf *Taskfile, arch ArchetypeInfo) Affinity {
	// Only Tools have affinity concerns
	if arch.Type != ArchetypeTool {
		return AffinityCross
	}

	// Check explicit XPLAT_AFFINITY first (future-proofing)
	if tf.HasVarValue("XPLAT_AFFINITY", "native") {
		return AffinityNative
	}
	if tf.HasVarValue("XPLAT_AFFINITY", "cross") {
		return AffinityCross
	}

	// Fall back to runtime-specific detection
	// Go: CGO=1 means native
	if tf.HasVarValue("_CGO", "1") {
		return AffinityNative
	}

	// Rust: Check for native flag (future)
	// if tf.HasVarValue("_NATIVE", "true") { return AffinityNative }

	// Default to cross-compile capable
	return AffinityCross
}

// ExeExt returns ".exe" on Windows, "" elsewhere.
// This is the single source of truth for executable extension handling.
func ExeExt() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// ExtractBinaryName gets the binary name with platform extension from a Taskfile.
// Returns empty string if no _BIN var found.
func ExtractBinaryName(tf *Taskfile) string {
	_, binValue, found := tf.GetVarBySuffix("_BIN")
	if found && binValue != "" {
		binName := binValue
		binName = strings.ReplaceAll(binName, "{{exeExt}}", ExeExt())
		binName = strings.Trim(binName, "'\"")
		return binName
	}
	return ""
}

// ToolInfo provides comprehensive tool metadata extracted from a Taskfile.
type ToolInfo struct {
	Archetype  ArchetypeInfo
	Affinity   Affinity
	BinaryName string // With platform extension
	Version    string // From _VERSION var (may be template)
	Repo       string // From _REPO var
	CGO        bool   // From _CGO var
}

// ExtractToolInfo extracts comprehensive metadata from a Taskfile.
func ExtractToolInfo(tf *Taskfile) ToolInfo {
	arch := DetectArchetype(tf)
	affinity := DetectAffinity(tf, arch)

	return ToolInfo{
		Archetype:  arch,
		Affinity:   affinity,
		BinaryName: ExtractBinaryName(tf),
		Version:    extractVersion(tf),
		Repo:       extractRepo(tf),
		CGO:        extractCGO(tf),
	}
}

// extractVersion gets the version from _VERSION var suffix.
func extractVersion(tf *Taskfile) string {
	_, val, found := tf.GetVarBySuffix("_VERSION")
	if found {
		return val
	}
	return ""
}

// extractRepo gets the repo from _REPO var suffix.
func extractRepo(tf *Taskfile) string {
	_, val, found := tf.GetVarBySuffix("_REPO")
	if found {
		return val
	}
	return ""
}

// extractCGO determines CGO setting from _CGO var suffix.
func extractCGO(tf *Taskfile) bool {
	return tf.HasVarValue("_CGO", "1")
}
