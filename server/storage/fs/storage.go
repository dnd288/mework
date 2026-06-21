// Package fs implements the storage.Store interface using the local filesystem.
//
// Objects are stored as files under a configurable base directory:
//   <basePath>/<bucket>/<key>
//
// The fs driver is intended for development and testing. It emulates ETag
// (MD5 hex), last-modified (file modification time), and prefix listing
// (directory walk). Presigned URLs are not supported on this backend.
package fs

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"mework/server/storage"
	"mework/shared/core"
)

func init() {
	storage.Register(storage.DriverFS, func(cfg storage.Config) (storage.Store, error) {
		return NewStore(cfg.BasePath, cfg.Bucket)
	})
}

// Store implements storage.Store on the local filesystem.
type Store struct {
	basePath string
	bucket   string
}

// NewStore creates a filesystem-backed store rooted at basePath.
// If basePath is empty, a temporary directory is used.
func NewStore(basePath, bucket string) (*Store, error) {
	path := basePath
	if path == "" {
		path = filepath.Join(os.TempDir(), "mework-storage")
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return nil, fmt.Errorf("creating fs store root %s: %w", path, err)
	}
	return &Store{basePath: path, bucket: bucket}, nil
}

// objectPath returns the absolute filesystem path for the given key.
func (s *Store) objectPath(key string) string {
	return filepath.Join(s.basePath, s.bucket, filepath.Clean("/"+key))
}

// PutObject stores an object. The reader is consumed entirely.
func (s *Store) PutObject(ctx context.Context, ref core.ObjectRef, reader io.Reader) error {
	path := s.objectPath(ref.Key)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating directory for %s: %w", ref.Key, err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating file %s: %w", ref.Key, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, reader); err != nil {
		return fmt.Errorf("writing file %s: %w", ref.Key, err)
	}
	return nil
}

// GetObject retrieves an object's contents.
func (s *Store) GetObject(ctx context.Context, ref core.ObjectRef) (io.ReadCloser, error) {
	path := s.objectPath(ref.Key)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, core.ObjectDeleted
		}
		return nil, fmt.Errorf("opening %s: %w", ref.Key, err)
	}
	return f, nil
}

// HeadObject returns metadata for an object.
func (s *Store) HeadObject(ctx context.Context, ref core.ObjectRef) (core.ObjectInfo, error) {
	path := s.objectPath(ref.Key)
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return core.ObjectInfo{}, core.ObjectDeleted
		}
		return core.ObjectInfo{}, fmt.Errorf("stating %s: %w", ref.Key, err)
	}

	return core.ObjectInfo{
		Ref:          ref,
		Size:         info.Size(),
		ETag:         computeETag(path),
		LastModified: info.ModTime(),
	}, nil
}

// ListObjects returns objects whose key begins with the given prefix.
func (s *Store) ListObjects(ctx context.Context, prefix string) ([]core.ObjectInfo, error) {
	root := filepath.Join(s.basePath, s.bucket)
	var objects []core.ObjectInfo

	err := filepath.Walk(root, func(walkPath string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(root, walkPath)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		if prefix != "" && !strings.HasPrefix(rel, prefix) {
			return nil
		}

		objects = append(objects, core.ObjectInfo{
			Ref:          core.ObjectRef{Bucket: s.bucket, Key: rel},
			Size:         fi.Size(),
			ETag:         computeETag(walkPath),
			LastModified: fi.ModTime(),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing objects: %w", err)
	}

	sort.Slice(objects, func(i, j int) bool {
		return objects[i].Ref.Key < objects[j].Ref.Key
	})

	return objects, nil
}

// DeleteObject removes an object. Removing a non-existent file is not an error.
func (s *Store) DeleteObject(ctx context.Context, ref core.ObjectRef) error {
	path := s.objectPath(ref.Key)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("deleting %s: %w", ref.Key, err)
	}
	return nil
}

// PresignGetURL is not supported on the filesystem backend.
func (s *Store) PresignGetURL(ctx context.Context, ref core.ObjectRef, ttl time.Duration) (string, time.Time, error) {
	return "", time.Time{}, fmt.Errorf("presigned URLs not supported on fs backend")
}

// PresignPutURL is not supported on the filesystem backend.
func (s *Store) PresignPutURL(ctx context.Context, ref core.ObjectRef, ttl time.Duration) (string, time.Time, error) {
	return "", time.Time{}, fmt.Errorf("presigned URLs not supported on fs backend")
}

// PutMultipart uploads a large object by concatenating all parts into a single file.
// Each part is read entirely and written sequentially; the final ETag is the MD5
// of the concatenated content.
func (s *Store) PutMultipart(ctx context.Context, ref core.ObjectRef, parts []io.Reader) (string, error) {
	if len(parts) == 0 {
		return "", fmt.Errorf("no parts provided")
	}

	path := s.objectPath(ref.Key)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("creating directory for %s: %w", ref.Key, err)
	}

	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("creating file %s: %w", ref.Key, err)
	}
	defer f.Close()

	hasher := md5.New()
	multi := io.MultiWriter(f, hasher)

	for i, part := range parts {
		if _, err := io.Copy(multi, part); err != nil {
			return "", fmt.Errorf("writing part %d: %w", i+1, err)
		}
	}

	if err := f.Close(); err != nil {
		return "", fmt.Errorf("closing %s: %w", ref.Key, err)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// computeETag returns the MD5 hex of a file's contents.
func computeETag(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}
