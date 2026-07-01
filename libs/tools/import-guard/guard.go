// Package guard provides a CheckImport function that enforces the module-level
// dependency DAG for the mework monorepo.
//
// Module DAG (directed acyclic graph), keyed by the mework/libs/* module paths:
//
//	shared (leaf) — imports only stdlib + shared sub-packages
//	server — imports shared (and stdlib/external)
//	sandbox — imports shared (and stdlib/external)
//	client — imports shared + sandbox (and stdlib/external)
//
// Additional rules:
//   - No sandbox/engine/<X> imports another sandbox/engine/<Y>.
//   - shared imports no heavy third-party dependency.
package guard

import "strings"

// CheckImport returns true if an import from sourceMod to importPath is
// allowed by the module-level dependency DAG and engine-isolation rules.
func CheckImport(sourceMod, importPath string) bool {
	const (
		modShared  = "mework/libs/shared"
		modServer  = "mework/libs/server"
		modClient  = "mework/libs/client"
		modSandbox = "mework/libs/sandbox"
	)

	// belongsTo reports whether pkg is the module root or a sub-package of mod.
	belongsTo := func(pkg, mod string) bool {
		return pkg == mod || strings.HasPrefix(pkg, mod+"/")
	}

	// moduleOf classifies a package path to its canonical mework/libs/* module
	// root, accepting both the full module path (mework/libs/shared) and the
	// short logical form (mework/shared). Returns "" for non-mework packages.
	moduleOf := func(pkg string) string {
		for _, mod := range []string{modShared, modServer, modClient, modSandbox} {
			if belongsTo(pkg, mod) {
				return mod
			}
			// Short logical form: mework/<name> for mework/libs/<name>.
			short := "mework/" + mod[len("mework/libs/"):]
			if belongsTo(pkg, short) {
				return mod
			}
		}
		return ""
	}

	srcMod := moduleOf(sourceMod)

	// --- Engine-to-engine isolation ---
	// No engine subpackage may import a different engine's subpackage.
	const engineSeg = "/engine/"
	srcEngineAt := strings.Index(sourceMod, modSandbox+engineSeg)
	if srcMod == modSandbox && srcEngineAt >= 0 &&
		strings.Contains(importPath, modSandbox+engineSeg) {
		srcName := strings.SplitN(sourceMod[srcEngineAt+len(modSandbox+engineSeg):], "/", 2)[0]
		dstAt := strings.Index(importPath, modSandbox+engineSeg)
		dstName := strings.SplitN(importPath[dstAt+len(modSandbox+engineSeg):], "/", 2)[0]
		if srcName != dstName {
			return false
		}
	}

	// --- Non-mework imports are always allowed (stdlib, external deps) ---
	// unless the source is the shared module, which must not pull heavy
	// third-party dependencies.
	dstMod := moduleOf(importPath)

	if dstMod == "" {
		if srcMod == modShared {
			// shared: only standard library (no domain) and self.
			return !strings.Contains(importPath, ".")
		}
		return true
	}

	// --- Module DAG: enforce directed edges ---
	switch srcMod {
	case modShared:
		// shared → only self
		return dstMod == modShared

	case modServer:
		// server → only shared
		return dstMod == modShared

	case modSandbox:
		// sandbox → only shared
		return dstMod == modShared

	case modClient:
		// client → shared or sandbox
		return dstMod == modShared || dstMod == modSandbox

	default:
		// Unknown source module: allow defensively.
		return true
	}
}
