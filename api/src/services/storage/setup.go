// Package storage backs document attachments by GCS, with V4 signed URLs
// for direct browser uploads/downloads. The runtime service account on
// Cloud Run has no downloadable private key, so signing routes through
// the IAM Credentials API (SignBlob).
package storage

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"

	"cloud.google.com/go/compute/metadata"
	gcs "cloud.google.com/go/storage"
	"google.golang.org/api/iamcredentials/v1"
	"google.golang.org/api/iterator"

	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

// Config holds the GCS bucket configuration used to store copro documents.
type Config struct {
	// Bucket is the GCS bucket name.
	Bucket string `yaml:"bucket"`
	// SigningServiceAccount is the email of the SA whose key signs V4 URLs
	// (via iamcredentials.SignBlob). On Cloud Run this is the runtime SA;
	// when empty, the client tries the GCE metadata server. Set explicitly
	// in conf/local.yml for local dev — the user must hold
	// roles/iam.serviceAccountTokenCreator on it.
	SigningServiceAccount string `yaml:"signing_service_account"`
}

// Client wraps a GCS client tied to the configured bucket. Implements
// interfaces.StorageService so the expenses usecase can mock it.
type Client struct {
	storage *gcs.Client
	iam     *iamcredentials.Service
	bucket  string

	signingMu sync.Mutex
	signingSA string
}

// NewClient creates a GCS-backed storage client.
func NewClient(conf Config) (*Client, error) {
	ctx := context.Background()
	storageClient, err := gcs.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("storage: new gcs client: %w", err)
	}

	iamService, err := iamcredentials.NewService(ctx)
	if err != nil {
		_ = storageClient.Close()
		return nil, fmt.Errorf("storage: new iamcredentials service: %w", err)
	}

	return &Client{
		storage:   storageClient,
		iam:       iamService,
		bucket:    conf.Bucket,
		signingSA: conf.SigningServiceAccount,
	}, nil
}

// BucketName returns the configured bucket name.
func (c *Client) BucketName() string {
	return c.bucket
}

// Close releases the underlying GCS client.
func (c *Client) Close() error {
	return c.storage.Close()
}

// resolveSigningSA returns the SA email used for signing, falling back to
// the GCE metadata server when no static value was configured. Cached after
// first lookup.
func (c *Client) resolveSigningSA(ctx context.Context) (string, error) {
	c.signingMu.Lock()
	defer c.signingMu.Unlock()
	if c.signingSA != "" {
		return c.signingSA, nil
	}
	if metadata.OnGCE() {
		email, err := metadata.EmailWithContext(ctx, "default")
		if err == nil && email != "" {
			c.signingSA = email
			return email, nil
		}
	}
	return "", errors.New("storage: signing_service_account is not configured (set storage.signing_service_account in conf or run on GCE/Cloud Run)")
}

// signBytes is the V4 SignedURL callback. It hands the bytes-to-sign to
// iamcredentials.SignBlob, which signs them with the SA's Google-managed
// key — no downloadable key required.
func (c *Client) signBytes(sa string) func([]byte) ([]byte, error) {
	return func(b []byte) ([]byte, error) {
		resp, err := c.iam.Projects.ServiceAccounts.SignBlob(
			"projects/-/serviceAccounts/"+sa,
			&iamcredentials.SignBlobRequest{
				Payload: base64.StdEncoding.EncodeToString(b),
			},
		).Do()
		if err != nil {
			return nil, fmt.Errorf("storage: iam SignBlob: %w", err)
		}
		return base64.StdEncoding.DecodeString(resp.SignedBlob)
	}
}

// SignedPutURL returns a V4 signed URL the browser can PUT to. The
// Content-Type AND Content-Length-Range are signed in via the `Headers`
// list — the client MUST send matching headers (Content-Type, and
// `x-goog-content-length-range:0,<sizeBytes>`) or GCS rejects the upload at
// write time. This pins both the type and the size, so a malicious client
// can't declare `size=100` and PUT 10GB.
func (c *Client) SignedPutURL(ctx context.Context, objectName, contentType string, sizeBytes int64, ttl time.Duration) (string, error) {
	sa, err := c.resolveSigningSA(ctx)
	if err != nil {
		return "", err
	}
	if sizeBytes <= 0 {
		return "", fmt.Errorf("storage: signed put url: sizeBytes must be > 0")
	}
	headers := []string{
		"Content-Type:" + contentType,
		fmt.Sprintf("x-goog-content-length-range:0,%d", sizeBytes),
	}
	url, err := gcs.SignedURL(c.bucket, objectName, &gcs.SignedURLOptions{
		Scheme:         gcs.SigningSchemeV4,
		Method:         "PUT",
		GoogleAccessID: sa,
		SignBytes:      c.signBytes(sa),
		ContentType:    contentType,
		Headers:        headers,
		Expires:        time.Now().Add(ttl),
	})
	if err != nil {
		return "", fmt.Errorf("storage: signed put url: %w", err)
	}
	return url, nil
}

// SignedGetURL returns a short-lived V4 signed URL for reading the object.
func (c *Client) SignedGetURL(ctx context.Context, objectName string, ttl time.Duration) (string, error) {
	sa, err := c.resolveSigningSA(ctx)
	if err != nil {
		return "", err
	}
	url, err := gcs.SignedURL(c.bucket, objectName, &gcs.SignedURLOptions{
		Scheme:         gcs.SigningSchemeV4,
		Method:         "GET",
		GoogleAccessID: sa,
		SignBytes:      c.signBytes(sa),
		Expires:        time.Now().Add(ttl),
	})
	if err != nil {
		return "", fmt.Errorf("storage: signed get url: %w", err)
	}
	return url, nil
}

// Head returns (stat, true, nil) when the object exists. Returns (zero,
// false, nil) when absent.
func (c *Client) Head(ctx context.Context, objectName string) (interfaces.ObjectStat, bool, error) {
	attrs, err := c.storage.Bucket(c.bucket).Object(objectName).Attrs(ctx)
	if err != nil {
		if errors.Is(err, gcs.ErrObjectNotExist) {
			return interfaces.ObjectStat{}, false, nil
		}
		return interfaces.ObjectStat{}, false, fmt.Errorf("storage: head %q: %w", objectName, err)
	}
	return interfaces.ObjectStat{
		SizeBytes:   attrs.Size,
		ContentType: attrs.ContentType,
	}, true, nil
}

// Delete removes a single object. Idempotent — missing objects are no-ops.
func (c *Client) Delete(ctx context.Context, objectName string) error {
	if err := c.storage.Bucket(c.bucket).Object(objectName).Delete(ctx); err != nil {
		if errors.Is(err, gcs.ErrObjectNotExist) {
			return nil
		}
		return fmt.Errorf("storage: delete %q: %w", objectName, err)
	}
	return nil
}

// DeletePrefix removes every object under the given prefix. Used by the
// expense-delete cascade. Best-effort: iterates the full listing and
// continues past per-object errors so a single transient failure doesn't
// leave half the prefix behind. Returns the first error encountered (if
// any) once the iteration completes.
func (c *Client) DeletePrefix(ctx context.Context, prefix string) error {
	it := c.storage.Bucket(c.bucket).Objects(ctx, &gcs.Query{Prefix: prefix})
	var firstErr error
	for {
		attrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("storage: list %q: %w", prefix, err)
			}
			break
		}
		if err := c.Delete(ctx, attrs.Name); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
