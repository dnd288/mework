// Package workspacefs provides the agent-facing file-system view of a session
// workspace. ReadFile / List / Stat may read across the shared root;
// WriteFile / Remove are confined to the grant's writable prefix. All paths
// are normalized and ".." traversal is blocked.
package workspacefs

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileInfo holds metadata about a file in the workspace.
type FileInfo struct {
	Name    string
	Size    int64
	IsDir   bool
	ModTime time.Time
	Mode    os.FileMode
}

// WorkspaceFS is the agent-facing file-system view of a workspace.
//
// ReadFile, List, and Stat may read across the shared root.
// WriteFile and Remove are confined to the workspace's writable prefix.
// All paths are normalized and ".." traversal is blocked — a write resolving
// outside the writable prefix (including via traversal) is denied.
type WorkspaceFS interface {
	ReadFile(ctx context.Context, path string) (io.ReadCloser, error)
	WriteFile(ctx context.Context, path string, reader io.Reader) error
	List(ctx context.Context, dir string) ([]FileInfo, error)
	Remove(ctx context.Context, path string) error
	Stat(ctx context.Context, path string) (FileInfo, error)
}

// LocalWorkspaceFS is a local-filesystem implementation of WorkspaceFS.
// It enforces path traversal blocking and write confinement to a granted prefix.
type LocalWorkspaceFS struct {
	rootDir        string
	writablePrefix string
	sharedRoots    []string
}

// NewLocal creates a new LocalWorkspaceFS rooted at rootDir.
//
// writablePrefix restricts writes to that sub-path (empty means the full root
// is writable). sharedRoots are additional read-only directories whose contents
// are visible to ReadFile, List, and Stat.
func NewLocal(rootDir string, writablePrefix string, sharedRoots []string) *LocalWorkspaceFS {
	return &LocalWorkspaceFS{
		rootDir:        filepath.Clean(rootDir),
		writablePrefix: writablePrefix,
		sharedRoots:    sharedRoots,
	}
}

// safePath resolves a user-supplied path relative to the workspace root and
// validates it against traversal and (for writes) writable-prefix confinement.
func (fs *LocalWorkspaceFS) safePath(path string, forWrite bool) (string, error) {
	// Normalize: clean the path and ensure it is relative.
	clean := filepath.Clean("/" + path)
	clean = strings.TrimPrefix(clean, "/")

	resolved := filepath.Join(fs.rootDir, clean)

	// Block ".." traversal: ensure resolved path is within rootDir.
	rootClean := filepath.Clean(fs.rootDir)
	if !strings.HasPrefix(resolved, rootClean+string(filepath.Separator)) && resolved != rootClean {
		return "", fmt.Errorf("path traversal blocked: %s", path)
	}

	// For writes, confine to the writable prefix.
	if forWrite && fs.writablePrefix != "" {
		prefixPath := filepath.Join(fs.rootDir, fs.writablePrefix)
		prefixClean := filepath.Clean(prefixPath)
		if !strings.HasPrefix(resolved, prefixClean+string(filepath.Separator)) && resolved != prefixClean {
			return "", fmt.Errorf("write denied: path %s is outside writable prefix %s", path, fs.writablePrefix)
		}
	}

	return resolved, nil
}

// ReadFile reads a file from the workspace or shared roots.
// It tries the workspace root first; if the file is not found there,
// it searches each shared root in order.
func (fs *LocalWorkspaceFS) ReadFile(ctx context.Context, path string) (io.ReadCloser, error) {
	resolved, err := fs.safePath(path, false)
	if err != nil {
		return nil, err
	}

	// Try the workspace root.
	f, err := os.Open(resolved)
	if err == nil {
		return f, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}

	// Fall through to shared roots (read-only).
	for _, root := range fs.sharedRoots {
		sharedPath := filepath.Join(root, path)
		sharedClean := filepath.Clean(sharedPath)
		rootClean := filepath.Clean(root)
		if !strings.HasPrefix(sharedClean, rootClean+string(filepath.Separator)) && sharedClean != rootClean {
			continue
		}
		f, err := os.Open(sharedClean)
		if err == nil {
			return f, nil
		}
	}

	return nil, fs.ErrNotExist(path)
}

// WriteFile writes a file into the workspace, confined to the writable prefix.
// Parent directories are created as needed.
func (fs *LocalWorkspaceFS) WriteFile(ctx context.Context, path string, reader io.Reader) error {
	resolved, err := fs.safePath(path, true)
	if err != nil {
		return err
	}

	// Ensure parent directory exists.
	parent := filepath.Dir(resolved)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return fmt.Errorf("mkdir parent: %w", err)
	}

	f, err := os.Create(resolved)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, reader); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

// List returns directory entries for the given directory.
// It checks the workspace root first, then falls through to shared roots.
func (fs *LocalWorkspaceFS) List(ctx context.Context, dir string) ([]FileInfo, error) {
	resolved, err := fs.safePath(dir, false)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(resolved)
	if err == nil {
		return toFileInfos(entries), nil
	}

	// Fall through to shared roots.
	for _, root := range fs.sharedRoots {
		sharedPath := filepath.Join(root, dir)
		sharedClean := filepath.Clean(sharedPath)
		rootClean := filepath.Clean(root)
		if !strings.HasPrefix(sharedClean, rootClean+string(filepath.Separator)) && sharedClean != rootClean {
			continue
		}
		entries, err := os.ReadDir(sharedClean)
		if err == nil {
			return toFileInfos(entries), nil
		}
	}

	return nil, err
}

// toFileInfos converts os.DirEntry slices to FileInfo slices.
func toFileInfos(entries []os.DirEntry) []FileInfo {
	infos := make([]FileInfo, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		infos = append(infos, FileInfo{
			Name:    e.Name(),
			Size:    info.Size(),
			IsDir:   e.IsDir(),
			ModTime: info.ModTime(),
			Mode:    info.Mode(),
		})
	}
	return infos
}

// Remove deletes a file or directory tree within the writable prefix.
func (fs *LocalWorkspaceFS) Remove(ctx context.Context, path string) error {
	resolved, err := fs.safePath(path, true)
	if err != nil {
		return err
	}

	return os.RemoveAll(resolved)
}

// Stat returns file metadata for the given path.
// It checks the workspace root first, then falls through to shared roots.
func (fs *LocalWorkspaceFS) Stat(ctx context.Context, path string) (FileInfo, error) {
	resolved, err := fs.safePath(path, false)
	if err != nil {
		return FileInfo{}, err
	}

	info, err := os.Stat(resolved)
	if err == nil {
		return toFileInfo(info), nil
	}

	// Fall through to shared roots.
	for _, root := range fs.sharedRoots {
		sharedPath := filepath.Join(root, path)
		sharedClean := filepath.Clean(sharedPath)
		rootClean := filepath.Clean(root)
		if !strings.HasPrefix(sharedClean, rootClean+string(filepath.Separator)) && sharedClean != rootClean {
			continue
		}
		info, err := os.Stat(sharedClean)
		if err == nil {
			return toFileInfo(info), nil
		}
	}

	return FileInfo{}, err
}

// toFileInfo converts a fs.FileInfo to workspacefs.FileInfo.
func toFileInfo(info fs.FileInfo) FileInfo {
	return FileInfo{
		Name:    info.Name(),
		Size:    info.Size(),
		IsDir:   info.IsDir(),
		ModTime: info.ModTime(),
		Mode:    info.Mode(),
	}
}

// ErrNotExist returns a standard "file not found" error for the given path.
func (fs *LocalWorkspaceFS) ErrNotExist(path string) error {
	return fmt.Errorf("workspace file not found: %s", path)
}
