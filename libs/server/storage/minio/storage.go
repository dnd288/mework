// Package minio implements storage.Store using MinIO. MinIO is S3-compatible,
// so this driver uses the shared s3compat package with a configurable endpoint.
package minio

import (
	"context"
	"fmt"
	"io"
	"time"

	"mework/libs/server/storage"
	"mework/libs/server/storage/s3compat"
	"mework/libs/shared/core"
)

func init() {
	storage.Register(storage.DriverMinIO, func(cfg storage.Config) (storage.Store, error) {
		return NewStore(cfg)
	})
}

// Store implements storage.Store against a MinIO endpoint.
type Store struct {
	client *s3compat.Client
	bucket string
}

// NewStore creates a new MinIO-backed store from the given config.
func NewStore(cfg storage.Config) (*Store, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("minio driver requires an endpoint")
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("minio driver requires a bucket name")
	}

	client := s3compat.NewClient(s3compat.Config{
		Endpoint:  cfg.Endpoint,
		Region:    cfg.Region,
		Bucket:    cfg.Bucket,
		AccessKey: cfg.Credentials.AccessKey,
		SecretKey: cfg.Credentials.SecretKey,
	})

	return &Store{client: client, bucket: cfg.Bucket}, nil
}

func (s *Store) PutObject(ctx context.Context, ref core.ObjectRef, reader io.Reader) error {
	_, err := s.client.PutObject(ctx, ref.Key, reader)
	return err
}

func (s *Store) GetObject(ctx context.Context, ref core.ObjectRef) (io.ReadCloser, error) {
	return s.client.GetObject(ctx, ref.Key)
}

func (s *Store) HeadObject(ctx context.Context, ref core.ObjectRef) (core.ObjectInfo, error) {
	return s.client.HeadObject(ctx, ref.Key)
}

func (s *Store) ListObjects(ctx context.Context, prefix string) ([]core.ObjectInfo, error) {
	return s.client.ListObjects(ctx, prefix)
}

func (s *Store) DeleteObject(ctx context.Context, ref core.ObjectRef) error {
	return s.client.DeleteObject(ctx, ref.Key)
}

func (s *Store) PresignGetURL(ctx context.Context, ref core.ObjectRef, ttl time.Duration) (string, time.Time, error) {
	return s.client.PresignGetURL(ctx, ref.Key, ttl)
}

func (s *Store) PresignPutURL(ctx context.Context, ref core.ObjectRef, ttl time.Duration) (string, time.Time, error) {
	return s.client.PresignPutURL(ctx, ref.Key, ttl)
}

func (s *Store) PutMultipart(ctx context.Context, ref core.ObjectRef, parts []io.Reader) (string, error) {
	return s.client.PutMultipart(ctx, ref.Key, parts)
}
// Delete deletes an object (implements storage.Store).
func (s *Store) Delete(ctx context.Context, ref core.ObjectRef) error {
	return s.DeleteObject(ctx, ref)
}

// Get retrieves an object (implements storage.Store).
func (s *Store) Get(ctx context.Context, ref core.ObjectRef) (io.ReadCloser, error) {
	return s.GetObject(ctx, ref)
}

// List lists objects (implements storage.Store).
func (s *Store) List(ctx context.Context, prefix string) ([]core.ObjectInfo, error) {
	return s.ListObjects(ctx, prefix)
}

// Put stores an object (implements storage.Store).
func (s *Store) Put(ctx context.Context, ref core.ObjectRef, reader io.Reader) error {
	return s.PutObject(ctx, ref, reader)
}

