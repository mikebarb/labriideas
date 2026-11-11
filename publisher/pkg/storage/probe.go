package storage

// ─────────────────────────────────────────────────────────────────────────
// PROBE-ONLY PRIMITIVES
//
// Everything in this file is used only by cmd/r2-metadata-probe, a
// developer tool. None of it is used by the server or the shipped CLI
// tools, and it is deliberately kept in pkg/storage (rather than in the
// tool) so the probe gets the s3 client without that field needing to be
// exported — a wider export would invite production code to bypass the
// Client's own methods.
//
// The distinction the probe exists to resolve: the historical comment in
// client.go claims PutObject preserves metadata bytes verbatim while
// CopyObject applies RFC 2047 encoding. The audit found 288 values
// containing encoded-word artefacts. These primitives let us test both
// paths against a literal "?" and find out which claim holds.
// ─────────────────────────────────────────────────────────────────────────

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// PutForProbe uploads a byte slice with optional metadata and optional
// Content-Type. Unlike UploadFile it does not lowercase keys — the probe
// must be able to send a deliberately mixed-case or non-ASCII key to
// observe what R2 does with it.
func (c *Client) PutForProbe(ctx context.Context, key string, data []byte, metadata map[string]string, contentType string) error {
	in := &s3.PutObjectInput{
		Bucket:        aws.String(c.bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(data),
		ContentLength: aws.Int64(int64(len(data))),
	}
	if len(metadata) > 0 {
		in.Metadata = metadata
	}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	_, err := c.s3.PutObject(ctx, in)
	return err
}

// CopyForProbe performs CopyObject over an object onto itself.
//
// replace=false sends no metadata directive at all, which is the control
// case: it should leave metadata untouched. replace=true sends REPLACE
// with the supplied map, which is the behaviour the old UpdateMetadata_OLD
// used. contentType is applied only with REPLACE, mirroring how the real
// copy path behaved.
//
// The CopySource key MUST be URL-path-escaped (spaces, "?", accented
// characters), and the destination key must NOT be — they occupy
// different positions in the request and are parsed differently. This
// asymmetry is the single most common source of CopyObject failures.
func (c *Client) CopyForProbe(ctx context.Context, key string, metadata map[string]string, replace bool, contentType string) error {
	in := &s3.CopyObjectInput{
		Bucket:     aws.String(c.bucket),
		Key:        aws.String(key),
		CopySource: aws.String("/" + c.bucket + "/" + url.PathEscape(key)),
	}
	if replace {
		in.MetadataDirective = types.MetadataDirectiveReplace
		in.Metadata = metadata
		if contentType != "" {
			in.ContentType = aws.String(contentType)
		}
	}
	_, err := c.s3.CopyObject(ctx, in)
	return err
}

// PresignedProbe is what the probe needs to know about a presigned
// request: the URL, the method, and — critically — WHICH HEADERS WERE
// SIGNED. If x-amz-meta-* appears in SignedHeader, a client may send
// metadata on a presigned PUT and the signature will validate. If it does
// not appear, sending those headers produces SignatureDoesNotMatch and
// Option A cannot perform metadata writes via presigning at all.
type PresignedProbe struct {
	URL          string
	Method       string
	SignedHeader http.Header
}

// PresignForProbe signs a GET, HEAD or PUT for key.
//
// For PUT, metadata and contentType are included in the signing input —
// which is exactly what we are testing. expires is capped by R2/SigV4 at
// 7 days; the server uses 15 minutes for uploads.
//
// NOTE ON TYPES: every Presign* method on the presign client returns
// *v4.PresignedHTTPRequest from the signer package
// (github.com/aws/aws-sdk-go-v2/aws/signer/v4) — there is no
// per-operation output type in service/s3. The struct carries URL,
// Method and SignedHeader, and SignedHeader is the field that answers
// the Option A question: if x-amz-meta-* appears in it, a client may
// send metadata headers on a presigned PUT and the signature validates.
func (c *Client) PresignForProbe(ctx context.Context, key, method string, metadata map[string]string, contentType string, expires time.Duration) (*PresignedProbe, error) {
	presigner := s3.NewPresignClient(c.s3)
	opt := s3.WithPresignExpires(expires)

	var (
		urlStr string
		hdr    http.Header
		err    error
	)

	switch method {
	case http.MethodGet:
		var r *v4.PresignedHTTPRequest
		r, err = presigner.PresignGetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(c.bucket),
			Key:    aws.String(key),
		}, opt)
		if r != nil {
			urlStr, hdr = r.URL, r.SignedHeader
		}
	case http.MethodHead:
		var r *v4.PresignedHTTPRequest
		r, err = presigner.PresignHeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(c.bucket),
			Key:    aws.String(key),
		}, opt)
		if r != nil {
			urlStr, hdr = r.URL, r.SignedHeader
		}
	case http.MethodPut:
		in := &s3.PutObjectInput{
			Bucket: aws.String(c.bucket),
			Key:    aws.String(key),
		}
		if len(metadata) > 0 {
			in.Metadata = metadata
		}
		if contentType != "" {
			in.ContentType = aws.String(contentType)
		}
		var r *v4.PresignedHTTPRequest
		r, err = presigner.PresignPutObject(ctx, in, opt)
		if r != nil {
			urlStr, hdr = r.URL, r.SignedHeader
		}
	default:
		err = errUnsupportedMethod(method)
	}

	if err != nil {
		return nil, err
	}
	return &PresignedProbe{URL: urlStr, Method: method, SignedHeader: hdr}, nil
}

type errUnsupportedMethod string

func (e errUnsupportedMethod) Error() string { return "unsupported presign method: " + string(e) }

// ─────────────────────────────────────────────────────────────────────────
// BYTE-COUNTING TRANSPORT
// ─────────────────────────────────────────────────────────────────────────

// CountingTransport wraps an http.RoundTripper and accumulates the bytes
// actually written to and read from the network.
//
// This is how the probe answers "does CopyObject move the object body?"
// with a number instead of an assumption: a server-side copy of a 2 MB
// object should transfer a small XML response and essentially no request
// body, whereas Get+Put transfers roughly twice the object size.
type CountingTransport struct {
	base http.RoundTripper

	mu            sync.Mutex
	requests      int64
	requestBytes  int64
	responseBytes int64
}

// NewCountingTransport wraps base (nil means http.DefaultTransport).
func NewCountingTransport(base http.RoundTripper) *CountingTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &CountingTransport{base: base}
}

// Snapshot returns the counters accumulated so far.
func (t *CountingTransport) Snapshot() (requests, reqBytes, respBytes int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.requests, t.requestBytes, t.responseBytes
}

// Reset zeroes the counters — the probe resets between cases so each
// case's cost is attributable.
func (t *CountingTransport) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.requests, t.requestBytes, t.responseBytes = 0, 0, 0
}

func (t *CountingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		req.Body = &countingReadCloser{rc: req.Body, mu: &t.mu, counter: &t.requestBytes}
	}

	resp, err := t.base.RoundTrip(req)

	t.mu.Lock()
	t.requests++
	t.mu.Unlock()

	if err != nil {
		return nil, err
	}
	if resp.Body != nil {
		resp.Body = &countingReadCloser{rc: resp.Body, mu: &t.mu, counter: &t.responseBytes}
	}
	return resp, nil
}

type countingReadCloser struct {
	rc      io.ReadCloser
	mu      *sync.Mutex
	counter *int64
}

func (c *countingReadCloser) Read(p []byte) (int, error) {
	n, err := c.rc.Read(p)
	if n > 0 {
		c.mu.Lock()
		*c.counter += int64(n)
		c.mu.Unlock()
	}
	return n, err
}

func (c *countingReadCloser) Close() error { return c.rc.Close() }
