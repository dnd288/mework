// Package s3 implements storage.Store using Amazon S3 or any S3-compatible
// endpoint. It uses the shared s3compat package for SigV4 signing and HTTP
// transport, and adds no external SDK dependency.
package s3

import (
	"context"
	"fmt"
	"io"
	"time"

	"mework/server/storage"
	"mework/server/storage/s3compat"
	"mework/shared/core"
)

func init() {
	storage.Register(storage.DriverS3, func(cfg storage.Config) (storage.Store, error) {
		return NewStore(cfg)
	})
}

// Store implements storage.Store against an S3-compatible endpoint.
type Store struct {
	client *s3compat.Client
	bucket string
}

// NewStore creates a new S3-backed store from the given config.
func NewStore(cfg storage.Config) (*Store, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("s3 driver requires an endpoint")
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("s3 driver requires a bucket name")
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
