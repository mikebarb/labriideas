package storage

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// ListObjectKeys returns every object key in the bucket, paginated.
//
// This is the read-only primitive the encoding audit (and any future
// bucket-wide tool) needs. It deliberately lists ALL objects rather than
// filtering to .mp3: the audit must see probe leftovers, manifests, and
// any unexpected keys, because "unexpected key in the bucket" is itself
// an audit finding. Callers filter.
//
// The pagination pattern mirrors CrawlCatalog's ListObjectsV2Paginator
// loop — extracted here so future tools reuse one implementation instead
// of copying the loop again. CrawlCatalog itself is left untouched in
// this pass (refactor-not-rewrite); it can adopt this method later.
func (c *Client) ListObjectKeys(ctx context.Context) ([]string, error) {
	var keys []string

	paginator := s3.NewListObjectsV2Paginator(c.s3, &s3.ListObjectsV2Input{
		Bucket: aws.String(c.bucket),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, obj := range page.Contents {
			// The SDK types Key as *string; it is nil-safe in practice
			// (ListObjectsV2 always populates it), but guard the
			// dereference rather than assume — a nil here would panic
			// the whole audit run for one malformed listing page.
			if obj.Key == nil {
				continue
			}
			key := *obj.Key

			// Skip folder markers: R2 (like S3) can materialise a
			// zero-byte object whose key ends in "/" when a console
			// "create folder" action is used. It has no metadata to
			// audit and no meaningful filename.
			if len(key) > 0 && key[len(key)-1] == '/' {
				continue
			}
			keys = append(keys, key)
		}
	}

	return keys, nil
}
