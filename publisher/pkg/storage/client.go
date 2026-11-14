package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"net/url"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/mikebarb/labriideas-publisher/pkg/metadata"
)

// Client is our core wrapper around the S3/R2 SDK
type Client struct {
	s3     *s3.Client
	bucket string
}

// NewClient creates a new storage client.
// It takes an already-configured S3 client and the bucket name.
func NewClient(s3Client *s3.Client, bucketName string) *Client {
	return &Client{
		s3:     s3Client,
		bucket: bucketName,
	}
}

// Helper method to get the bucket name internally
func (c *Client) Bucket() string {
	return c.bucket
}

// GetDownloadURL generates a time-limited presigned URL for a file.
func (c *Client) GetDownloadURL(ctx context.Context, key string, expires time.Duration) (string, error) {
	presigner := s3.NewPresignClient(c.s3)

	presignedReq, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expires))

	if err != nil {
		return "", err
	}

	return presignedReq.URL, nil
}

// GetUploadURL generates a time-limited presigned URL for uploading a file.
func (c *Client) GetUploadURL(ctx context.Context, key string, expires time.Duration) (string, error) {
	presigner := s3.NewPresignClient(c.s3)

	presignedReq, err := presigner.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expires))

	if err != nil {
		return "", err
	}

	return presignedReq.URL, nil
}

// GetMetadata fetches only the headers/metadata for a file (Used for ETag checking)
func (c *Client) GetMetadata(ctx context.Context, key string) (*s3.HeadObjectOutput, error) {
	return c.s3.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
}

// GetObjectBytes downloads the entire file into a byte slice (Used for caching the catalog)
func (c *Client) GetObjectBytes(ctx context.Context, key string) ([]byte, error) {
	resp, err := c.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

// UpdateMetadata replaces the custom metadata for an existing object in
// R2, using a server-side CopyObject onto the object itself.
//
// ─── WHY COPY, NOT GET+PUT ───
// The previous implementation downloaded the whole object into the
// process and re-uploaded it. The probe measured a 1 MB CopyObject
// REPLACE at ONE byte of request body — the object body never leaves
// R2. On the hosted server this is the difference between ~30 MB and
// ~0 KB of billed bandwidth per metadata edit; it is also why the
// 2,000-file bulk run of November 2026 consumed 35.51 GB and suspended
// the workspace.
//
// ─── THE WRITE CONTRACT (probe-established) ───
//   - values are sent RAW: this function writes the merged map verbatim.
//     R2 normalises before storage; pre-encoding is WRONG (it would be
//     decoded into the literal encoded-word text).
//   - the caller's `changes` map is in Unicode space and uses the
//     empty-means-delete convention (see metadata.Merge).
//   - existing metadata is DECODED on read before merging — never merge
//     in encoded space.
//   - Content-Type is preserved from the existing object. The probe
//     (case 7) showed that omitting it demotes the object to
//     application/octet-stream, which is how 2,027 library objects lost
//     audio/mpeg. Preserving (not inventing) keeps this function
//     non-destructive; the repair pass fixes the historical damage.
//   - a verification HEAD follows every write: decoded stored values
//     must equal the expected merged map, and Content-Type must be
//     non-empty. This is what would have caught both historical
//     defects on day one.
//
// NOTE: the parameter is named `changes` rather than `metadata` because
// the package `metadata` is used directly in this function's body — a
// parameter of that name would shadow the package and fail to compile.
func (c *Client) UpdateMetadata(ctx context.Context, filename string, changes map[string]string) error {
	// 1. Read current state: metadata (in encoded transit form) and the
	//    Content-Type we must preserve.
	head, err := c.s3.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(filename),
	})
	if err != nil {
		return fmt.Errorf("failed to read current metadata for %s: %w", filename, err)
	}

	contentType := ""
	if head.ContentType != nil {
		contentType = *head.ContentType
	}

	// 2. Decode existing values into Unicode space, then merge the
	//    caller's changes (empty value deletes; absent preserves).
	existing := make(map[string]string, len(head.Metadata))
	for k, v := range head.Metadata {
		existing[k] = metadata.Decode(v)
	}
	merged := metadata.Merge(existing, changes)

	// 3. Copy-over-self with REPLACE. The merged map is the COMPLETE
	//    intended metadata set — REPLACE wipes anything not sent, which
	//    is exactly the deletion mechanism for fields removed from the
	//    merged map.
	in := &s3.CopyObjectInput{
		Bucket:            aws.String(c.bucket),
		Key:               aws.String(filename),
		CopySource:        aws.String("/" + c.bucket + "/" + url.PathEscape(filename)),
		MetadataDirective: types.MetadataDirectiveReplace,
		Metadata:          merged,
	}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	if _, err := c.s3.CopyObject(ctx, in); err != nil {
		return fmt.Errorf("failed to copy %s over itself for metadata update: %w", filename, err)
	}

	// 4. VERIFY. Trust nothing, including a nil error: HEAD the object
	//    and compare decoded stored values against the expected map.
	newHead, err := c.s3.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(filename),
	})
	if err != nil {
		return fmt.Errorf("metadata update for %s succeeded but verification HEAD failed: %w", filename, err)
	}

	decoded := make(map[string]string, len(newHead.Metadata))
	for k, v := range newHead.Metadata {
		decoded[k] = metadata.Decode(v)
	}
	if len(decoded) != len(merged) {
		return fmt.Errorf("metadata verification failed for %s: expected %d field(s), stored %d",
			filename, len(merged), len(decoded))
	}
	for k, want := range merged {
		if got, ok := decoded[k]; !ok {
			return fmt.Errorf("metadata verification failed for %s: field %q missing after write", filename, k)
		} else if got != want {
			return fmt.Errorf("metadata verification failed for %s: field %q: sent %q, stored %q",
				filename, k, want, got)
		}
	}
	// Invariant guard: no empty stored value should ever exist ("R2 is
	// sparse"), and Content-Type must never be lost to a metadata edit.
	for k, v := range decoded {
		if v == "" {
			return fmt.Errorf("metadata verification failed for %s: field %q stored empty", filename, k)
		}
	}
	if newHead.ContentType == nil || *newHead.ContentType == "" {
		return fmt.Errorf("metadata verification failed for %s: Content-Type missing after write", filename)
	}

	return nil
}

// PutObjectBytes uploads a byte slice to R2 at the specified key
func (c *Client) PutObjectBytes(ctx context.Context, key string, data []byte) error {
	reader := bytes.NewReader(data)

	_, err := c.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(key),
		ContentType: aws.String("application/gzip"),
		Body:        reader,
	})

	return err
}

// PutObjectWithMetadata uploads a byte slice to R2 and attaches custom metadata
func (c *Client) PutObjectWithMetadata(ctx context.Context, key string, data []byte, metadata map[string]string) error {
	reader := bytes.NewReader(data)

	_, err := c.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(key),
		Body:        reader,
		ContentType: aws.String("audio/mpeg"),
		Metadata:    metadata, // AWS SDK v2 expects standard map[string]string
	})

	return err
}
