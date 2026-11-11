package storage

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// NewClientFromEnv builds a storage.Client from the R2 environment
// variables, using the SAME variable names the server's main() reads:
//
//	R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY, R2_ACCOUNT_ID, R2_BUCKET_NAME
//
// WHY A DIRECT-CONNECTION CONSTRUCTOR, given the presign-broker (Option A)
// design: the broker covers the distributed CLI tools, which must never
// hold bucket credentials. But two classes of tool need DIRECT bucket
// access and are developer-only:
//
//   - the encoding audit, which must LIST the bucket — presigned URLs
//     cannot list, because you must already know a key to sign it; and
//   - the metadata probe, which must construct the same presigner the
//     server uses so its results are the server's results.
//
// These tools read the developer's .env (same names as the server's, so
// one .env can configure both) and are never shipped to administrators.
// Scope the R2 API token used here to this one bucket, object read/write.
//
// NOTE for callers: godotenv.Load() is NOT called here — the tool's main
// owns .env loading, matching how the server's main works.
func NewClientFromEnv(ctx context.Context) (*Client, error) {
	return newClientFromEnv(ctx, nil)
}

// NewClientFromEnvWithHTTPClient is identical except that the caller
// supplies the http.Client used by the SDK. The probe uses this to
// install a byte-counting transport: the only reliable way to answer
// "did the copy move the body or not" is to measure it, rather than
// infer it from the API call.
func NewClientFromEnvWithHTTPClient(ctx context.Context, hc *http.Client) (*Client, error) {
	return newClientFromEnv(ctx, hc)
}

func newClientFromEnv(ctx context.Context, hc *http.Client) (*Client, error) {
	accessKey := os.Getenv("R2_ACCESS_KEY_ID")
	secretKey := os.Getenv("R2_SECRET_ACCESS_KEY")
	accountID := os.Getenv("R2_ACCOUNT_ID")
	bucketName := os.Getenv("R2_BUCKET_NAME")

	if accessKey == "" || secretKey == "" || accountID == "" || bucketName == "" {
		return nil, fmt.Errorf(
			"R2 credentials incomplete — need R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY, " +
				"R2_ACCOUNT_ID and R2_BUCKET_NAME (check the .env beside this tool)",
		)
	}

	opts := []func(*config.LoadOptions) error{
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		config.WithRegion("auto"), // R2 requires the literal region "auto"
	}
	if hc != nil {
		opts = append(opts, config.WithHTTPClient(hc))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// R2's S3 endpoint is account-scoped: https://<account>.r2.cloudflarestorage.com
	s3Client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String("https://" + accountID + ".r2.cloudflarestorage.com")
	})

	return NewClient(s3Client, bucketName), nil
}
