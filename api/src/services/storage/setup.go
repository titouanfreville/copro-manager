package storage

import (
	"context"

	gcs "cloud.google.com/go/storage"
)

// Config holds the GCS bucket configuration used to store copro documents.
type Config struct {
	Bucket string `yaml:"bucket"`
}

// Client wraps a GCS client tied to the configured bucket.
type Client struct {
	storage *gcs.Client
	bucket  string
}

// NewClient creates a GCS-backed storage client.
func NewClient(conf Config) (*Client, error) {
	storageClient, err := gcs.NewClient(context.Background())
	if err != nil {
		return nil, err
	}

	return &Client{storage: storageClient, bucket: conf.Bucket}, nil
}

// Bucket returns the GCS bucket handle for the configured bucket.
func (c *Client) Bucket() *gcs.BucketHandle {
	return c.storage.Bucket(c.bucket)
}

// BucketName returns the configured bucket name.
func (c *Client) BucketName() string {
	return c.bucket
}

// Close releases the underlying GCS client.
func (c *Client) Close() error {
	return c.storage.Close()
}
