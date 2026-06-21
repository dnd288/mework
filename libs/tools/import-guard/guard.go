// Package guard provides a CheckImport function that enforces the module-level
// dependency DAG for the mework monorepo.
//
// Module DAG (directed acyclic graph):
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
		modShared  = "mework/shared"
		modServer  = "mework/server"
		modClient  = "mework/client"
		modSandbox = "mework/sandbox"
	)

	// belongsTo reports whether pkg is the module root or a sub-package of mod.
	belongsTo := func(pkg, mod string) bool {
		return pkg == mod || strings.HasPrefix(pkg, mod+"/")
	}

	// --- Engine-to-engine isolation ---
	// No engine subpackage may import a different engine's subpackage.
	if strings.HasPrefix(sourceMod, modSandbox+"/engine/") &&
		strings.HasPrefix(importPath, modSandbox+"/engine/") {
		srcName := strings.SplitN(sourceMod[len(modSandbox+"/engine/"):], "/", 2)[0]
		dstName := strings.SplitN(importPath[len(modSandbox+"/engine/"):], "/", 2)[0]
		if srcName != dstName {
			return false
		}
	}

	// --- Non-mework imports are always allowed (stdlib, external deps) ---
	// unless the source is the shared module, which must not pull heavy
	// third-party dependencies.
	isMework := belongsTo(importPath, modShared) ||
		belongsTo(importPath, modServer) ||
		belongsTo(importPath, modClient) ||
		belongsTo(importPath, modSandbox)

	if !isMework {
		if belongsTo(sourceMod, modShared) {
			// shared: only standard library (no domain) and self.
			return !strings.Contains(importPath, ".")
		}
		return true
	}

	// --- Module DAG: enforce directed edges ---
	switch {
	case belongsTo(sourceMod, modShared):
		// shared → only self
		return belongsTo(importPath, modShared)

	case belongsTo(sourceMod, modServer):
		// server → only shared
		return belongsTo(importPath, modShared)

	case belongsTo(sourceMod, modSandbox):
		// sandbox → only shared
		return belongsTo(importPath, modShared)

	case belongsTo(sourceMod, modClient):
		// client → shared or sandbox
		return belongsTo(importPath, modShared) || belongsTo(importPath, modSandbox)

	default:
		// Unknown source module: allow defensively.
		return true
	}
}
