package storage

import (
	"context"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
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
