package objectstore

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinIO struct {
	client *minio.Client
	bucket string
}

func NewMinIO(ctx context.Context, endpoint, accessKey, secretKey, bucket string, useSSL bool) (*MinIO, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}
	m := &MinIO{client: client, bucket: bucket}
	if err := m.ensureBucket(ctx); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *MinIO) ensureBucket(ctx context.Context) error {
	exists, err := m.client.BucketExists(ctx, m.bucket)
	if err != nil {
		return fmt.Errorf("check bucket: %w", err)
	}
	if exists {
		return nil
	}
	if err := m.client.MakeBucket(ctx, m.bucket, minio.MakeBucketOptions{}); err != nil {
		return fmt.Errorf("create bucket: %w", err)
	}
	return nil
}

func (m *MinIO) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	_, err := m.client.PutObject(ctx, m.bucket, key, r, size, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return fmt.Errorf("put %s: %w", key, err)
	}
	return nil
}

func (m *MinIO) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if _, err := m.client.StatObject(ctx, m.bucket, key, minio.StatObjectOptions{}); err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("stat %s: %w", key, err)
	}
	obj, err := m.client.GetObject(ctx, m.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", key, err)
	}
	return obj, nil
}

func (m *MinIO) Delete(ctx context.Context, key string) error {
	if err := m.client.RemoveObject(ctx, m.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("delete %s: %w", key, err)
	}
	return nil
}

func (m *MinIO) DeletePrefix(ctx context.Context, prefix string) error {
	objects := m.client.ListObjects(ctx, m.bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true})
	for err := range m.client.RemoveObjects(ctx, m.bucket, objects, minio.RemoveObjectsOptions{}) {
		if err.Err != nil {
			return fmt.Errorf("delete prefix %s: %w", prefix, err.Err)
		}
	}
	return nil
}

func (m *MinIO) ListKeys(ctx context.Context, prefix string) ([]string, error) {
	var keys []string
	for obj := range m.client.ListObjects(ctx, m.bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("list %s: %w", prefix, obj.Err)
		}
		keys = append(keys, obj.Key)
	}
	return keys, nil
}

func (m *MinIO) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	u, err := m.client.PresignedGetObject(ctx, m.bucket, key, ttl, url.Values{})
	if err != nil {
		return "", fmt.Errorf("presign %s: %w", key, err)
	}
	return u.String(), nil
}
