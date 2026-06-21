// Package storage defines the server-side object storage abstraction and
// provides the Store interface (a subset of ports.ObjectStore) and
// configuration types for selecting the backend driver.
//
// Drivers live in subpackages (s3, minio, r2, fs) and are selected via
// the Driver field of Config. Consumers import storage.Store and never
// a concrete driver package.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"mework/shared/core"
)

// Store is the server's object storage interface.
type Store interface {
	PutObject(ctx context.Context, ref core.ObjectRef, reader io.Reader) error
	GetObject(ctx context.Context, ref core.ObjectRef) (io.ReadCloser, error)
	HeadObject(ctx context.Context, ref core.ObjectRef) (core.ObjectInfo, error)
	ListObjects(ctx context.Context, prefix string) ([]core.ObjectInfo, error)
	DeleteObject(ctx context.Context, ref core.ObjectRef) error
	PresignGetURL(ctx context.Context, ref core.ObjectRef, ttl time.Duration) (string, time.Time, error)
	PresignPutURL(ctx context.Context, ref core.ObjectRef, ttl time.Duration) (string, time.Time, error)
	PutMultipart(ctx context.Context, ref core.ObjectRef, parts []io.Reader) (string, error)
}

// DriverName enumerates supported storage backends.
type DriverName string

const (
	DriverFS    DriverName = "fs"
	DriverS3    DriverName = "s3"
	DriverMinIO DriverName = "minio"
	DriverR2    DriverName = "r2"
)

// S3Credentials holds the access key and secret key for S3-compatible backends.
type S3Credentials struct {
	AccessKey string
	SecretKey string
}

// Config selects the storage backend and provides connection details.
type Config struct {
	// Driver selects the backend: "fs", "s3", "minio", or "r2".
	Driver DriverName

	// Endpoint is the S3-compatible endpoint URL (e.g. "https://s3.amazonaws.com").
	// For the fs driver this is empty.
	Endpoint string

	// Region is the AWS region (e.g. "us-east-1").
	Region string

	// Bucket is the default bucket name for object operations.
	Bucket string

	// Credentials for S3-compatible backends.
	Credentials S3Credentials

	// BasePath is the local filesystem root for the fs driver.
	// When empty, NewStore uses a temporary directory.
	BasePath string
}

// ErrUnsupportedDriver is returned when Config.Driver does not name a known backend.
var ErrUnsupportedDriver = errors.New("unsupported storage driver")

// Factory is a function that creates a Store from Config.
type Factory func(Config) (Store, error)

var drivers = make(map[DriverName]Factory)

// Register registers a storage driver factory for the given driver name.
// Called from driver init() functions.
func Register(name DriverName, fn Factory) {
	drivers[name] = fn
}

// NewStore creates a Store from config by selecting the appropriate driver.
func NewStore(cfg Config) (Store, error) {
	fn, ok := drivers[cfg.Driver]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedDriver, cfg.Driver)
	}
	return fn(cfg)
}
