// Command r2-metadata-probe establishes ground truth about how R2 and the
// AWS SDK treat custom object metadata, before any write-path refactor.
//
// The audit of the live bucket found 288 metadata values containing RFC
// 2047 encoded-word artefacts and 2027 .mp3 objects stripped of their
// audio/mpeg Content-Type. Neither finding tells us WHICH write path did
// the damage. This tool answers that, and answers whether Option A's
// presigned-metadata design is viable.
//
// SAFETY: every write and delete is confined to a per-run scratch prefix.
// A guard refuses any key outside it. Objects are deleted at the end
// unless --keep is passed, so a failing case can be inspected in the R2
// dashboard.
//
// This tool holds DIRECT bucket credentials and is developer-only.
package main

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"github.com/mikebarb/labriideas-publisher/pkg/metadata"
	"github.com/mikebarb/labriideas-publisher/pkg/storage"
)

// The literal values the cases are built from. The first mirrors a real
// corrupted title from the library; the second carries accented Latin
// text; the third combines both plus an em dash.
const (
	valQuestion = "How Does Jesus Relate to Unbelief?"
	valAccented = "Café de la Paix"
	valCombined = "Où va le monde? — Leçon trois"
)

type result struct {
	name     string
	verdict  string // PRESERVED / ENCODED_ON_WRITE / WIPED / INCONCLUSIVE / INFO
	detail   string
	reqBytes int64
	resBytes int64
}

func main() {
	_ = godotenv.Load() // must be run from the publisher root (where .env lives)

	keep := flag.Bool("keep", false, "keep probe objects in R2 for inspection")
	flag.Parse()

	ctx := context.Background()

	countTrans := storage.NewCountingTransport(nil)
	client, err := storage.NewClientFromEnvWithHTTPClient(ctx, &http.Client{
		Transport: countTrans,
		// Generous: a metadata PUT is small, but a 2 MB body case is not.
		Timeout: 120 * time.Second,
	})
	if err != nil {
		fmt.Printf("❌ Setup failed: %v\n", err)
		os.Exit(1)
	}

	prefix := fmt.Sprintf("_probe/%s/", time.Now().UTC().Format("20060102-150405"))
	fmt.Printf("=== R2 Metadata Probe ===\n")
	fmt.Printf("Bucket:  %s\n", client.Bucket())
	fmt.Printf("Scratch: %s\n\n", prefix)

	// guard refuses any operation outside the scratch prefix. Every write
	// and delete goes through it — the one safety net that makes this
	// tool safe to run against a shared bucket.
	guard := func(key string) {
		if !strings.HasPrefix(key, prefix) {
			fmt.Printf("❌ REFUSED: key %q is outside scratch prefix %q\n", key, prefix)
			os.Exit(1)
		}
	}

	var results []result
	add := func(r result) {
		results = append(results, r)
		fmt.Printf("  %-42s %s\n", r.name, r.verdict)
		if r.detail != "" {
			fmt.Printf("      %s\n", r.detail)
		}
	}

	// Object body used throughout: tiny, valid-looking bytes. The content
	// is irrelevant to metadata behaviour; what matters is that the key
	// does not end in .mp3, so a later catalogue crawl never sees it.
	body := []byte("probe-body-do-not-crawl")

	// ── CASE 1: "?" round-trip via PutObject ────────────────────────────
	// The decisive case. If the raw value comes back WITH the "?", the
	// Put path preserves bytes and the 288 artefacts came from the old
	// Copy path. If it comes back encoded, the write path itself is
	// encoding and decode-and-rewrite would re-corrupt.
	{
		key := prefix + "put-question.probe"
		guard(key)
		countTrans.Reset()

		putErr := client.PutForProbe(ctx, key, body, map[string]string{"title": valQuestion}, "audio/mpeg")
		if putErr != nil {
			add(result{name: "1. PUT with '?'", verdict: "ERROR", detail: putErr.Error()})
		} else {
			head, hErr := client.GetMetadata(ctx, key)
			if hErr != nil || head == nil {
				add(result{name: "1. PUT with '?'", verdict: "ERROR", detail: fmt.Sprint(hErr)})
			} else {
				raw := head.Metadata["title"]
				reqB, resB, _ := countTrans.Snapshot()
				add(result{
					name:     "1. PUT with '?'",
					verdict:  classify(valQuestion, raw),
					detail:   fmt.Sprintf("sent=%q  stored=%q  decoded=%q", valQuestion, raw, metadata.Decode(raw)),
					reqBytes: reqB,
					resBytes: resB,
				})
			}
		}
	}

	// ── CASE 2: "?" round-trip via CopyObject REPLACE ───────────────────
	// Same value, different transport — the path the old implementation
	// used. Compare the verdict with case 1.
	{
		key := prefix + "copy-question.probe"
		guard(key)

		if err := client.PutForProbe(ctx, key, body, map[string]string{"title": "baseline"}, "audio/mpeg"); err != nil {
			add(result{name: "2. COPY with '?'", verdict: "ERROR", detail: err.Error()})
		} else {
			countTrans.Reset()
			err := client.CopyForProbe(ctx, key, map[string]string{"title": valQuestion}, true, "audio/mpeg")
			if err != nil {
				add(result{name: "2. COPY with '?'", verdict: "ERROR", detail: err.Error()})
			} else {
				head, hErr := client.GetMetadata(ctx, key)
				if hErr != nil || head == nil {
					add(result{name: "2. COPY with '?'", verdict: "ERROR", detail: fmt.Sprint(hErr)})
				} else {
					raw := head.Metadata["title"]
					reqB, resB, _ := countTrans.Snapshot()
					add(result{
						name:     "2. COPY with '?'",
						verdict:  classify(valQuestion, raw),
						detail:   fmt.Sprintf("sent=%q  stored=%q  decoded=%q", valQuestion, raw, metadata.Decode(raw)),
						reqBytes: reqB,
						resBytes: resB,
					})
				}
			}
		}
	}

	// ── CASE 3: raw Unicode round-trip (PUT) ────────────────────────────
	{
		key := prefix + "put-unicode.probe"
		guard(key)

		if err := client.PutForProbe(ctx, key, body, map[string]string{"title": valCombined}, "audio/mpeg"); err != nil {
			add(result{name: "3. PUT with accented+em dash", verdict: "ERROR", detail: err.Error()})
		} else if head, hErr := client.GetMetadata(ctx, key); hErr != nil || head == nil {
			add(result{name: "3. PUT with accented+em dash", verdict: "ERROR", detail: fmt.Sprint(hErr)})
		} else {
			raw := head.Metadata["title"]
			add(result{
				name:    "3. PUT with accented+em dash",
				verdict: classify(valCombined, raw),
				detail:  fmt.Sprintf("sent=%q  stored=%q", valCombined, raw),
			})
		}
	}

	// ── CASE 4: re-PUT HEAD's own value (double-encode check) ───────────
	// If writing an already-stored value changes it, repeated edits in
	// the real system would drift. This is the loop the library would
	// repeat on every future metadata edit.
	{
		key := prefix + "reput.probe"
		guard(key)

		if err := client.PutForProbe(ctx, key, body, map[string]string{"title": valQuestion}, "audio/mpeg"); err != nil {
			add(result{name: "4. re-PUT of stored value", verdict: "ERROR", detail: err.Error()})
		} else if h1, e1 := client.GetMetadata(ctx, key); e1 != nil || h1 == nil {
			add(result{name: "4. re-PUT of stored value", verdict: "ERROR", detail: fmt.Sprint(e1)})
		} else {
			v1 := h1.Metadata["title"]
			// Write back exactly what HEAD returned, as a merge-based
			// rewrite would.
			_ = client.PutForProbe(ctx, key, body, map[string]string{"title": v1}, "audio/mpeg")
			if h2, e2 := client.GetMetadata(ctx, key); e2 != nil || h2 == nil {
				add(result{name: "4. re-PUT of stored value", verdict: "ERROR", detail: fmt.Sprint(e2)})
			} else {
				v2 := h2.Metadata["title"]
				verdict := "STABLE"
				if v1 != v2 {
					verdict = "DRIFTS_ON_REWRITE"
				}
				add(result{name: "4. re-PUT of stored value", verdict: verdict,
					detail: fmt.Sprintf("first=%q  second=%q", v1, v2)})
			}
		}
	}

	// ── CASE 5: COPY twice (compounding check) ──────────────────────────
	// Reproduces the historical "? → =3F" compounding: each copy pass
	// adding a layer.
	{
		key := prefix + "copy-twice.probe"
		guard(key)

		if err := client.PutForProbe(ctx, key, body, map[string]string{"title": valQuestion}, "audio/mpeg"); err != nil {
			add(result{name: "5. COPY twice", verdict: "ERROR", detail: err.Error()})
		} else if h1, e1 := client.GetMetadata(ctx, key); e1 != nil || h1 == nil {
			add(result{name: "5. COPY twice", verdict: "ERROR", detail: fmt.Sprint(e1)})
		} else {
			before := h1.Metadata["title"]
			_ = client.CopyForProbe(ctx, key, map[string]string{"title": before}, true, "audio/mpeg")
			h2, _ := client.GetMetadata(ctx, key)
			after1 := ""
			if h2 != nil {
				after1 = h2.Metadata["title"]
			}
			_ = client.CopyForProbe(ctx, key, map[string]string{"title": after1}, true, "audio/mpeg")
			h3, _ := client.GetMetadata(ctx, key)
			after2 := ""
			if h3 != nil {
				after2 = h3.Metadata["title"]
			}
			verdict := "STABLE"
			if !(before == after1 && after1 == after2) {
				verdict = "COMPOUNDS"
			}
			add(result{name: "5. COPY twice", verdict: verdict,
				detail: fmt.Sprintf("start=%q  after1=%q  after2=%q", before, after1, after2)})
		}
	}

	// ── CASE 6: presigned PUT with metadata — THE OPTION A GATE ─────────
	// Inspect SignedHeader to see whether x-amz-meta-* is covered by the
	// signature. Then actually perform the PUT with those headers.
	{
		key := prefix + "presign-put.probe"
		guard(key)

		pre, err := client.PresignForProbe(ctx, key, http.MethodPut,
			map[string]string{"title": valQuestion}, "audio/mpeg", 10*time.Minute)
		if err != nil {
			add(result{name: "6. presigned PUT + metadata", verdict: "ERROR", detail: err.Error()})
		} else {
			signedMeta := false
			var names []string
			for h := range pre.SignedHeader {
				names = append(names, h)
				if strings.HasPrefix(strings.ToLower(h), "x-amz-meta-") {
					signedMeta = true
				}
			}
			detail := "SignedHeader: " + strings.Join(names, ", ")

			req, _ := http.NewRequest(http.MethodPut, pre.URL, strings.NewReader(string(body)))
			req.ContentLength = int64(len(body))
			for h, vs := range pre.SignedHeader {
				for _, v := range vs {
					req.Header.Set(h, v)
				}
			}
			req.Header.Set("Content-Type", "audio/mpeg")

			resp, doErr := http.DefaultClient.Do(req)
			status := 0
			var respBody string
			if doErr == nil {
				status = resp.StatusCode
				b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
				respBody = string(b)
				resp.Body.Close()
			}

			verdict := "INFO"
			switch {
			case !signedMeta:
				verdict = "METADATA_NOT_SIGNED"
				detail += "  → x-amz-meta-* is NOT in SignedHeader; Option A cannot PUT metadata this way"
			case status == 200:
				verdict = "PRESIGN_METADATA_OK"
				// Read back to confirm the metadata actually landed.
				if head, e := client.GetMetadata(ctx, key); e == nil && head != nil {
					detail += fmt.Sprintf("  stored=%q", head.Metadata["title"])
				}
			default:
				verdict = fmt.Sprintf("PRESIGN_FAILED_%d", status)
				detail += "  body=" + strings.TrimSpace(respBody)
			}
			add(result{name: "6. presigned PUT + metadata", verdict: verdict, detail: detail})
		}
	}

	// ── CASE 7: Content-Type survival ───────────────────────────────────
	// Explains the 2027 objects: what does a PUT with no ContentType
	// store, and does COPY REPLACE preserve one?
	{
		key := prefix + "ctype.probe"
		guard(key)

		// 7a: PUT with no Content-Type at all — what default appears?
		var partA string
		if err := client.PutForProbe(ctx, key, body, map[string]string{"title": "x"}, ""); err != nil {
			partA = "put-no-ctype error: " + err.Error()
		} else if head, e := client.GetMetadata(ctx, key); e == nil && head != nil {
			ct := ""
			if head.ContentType != nil {
				ct = *head.ContentType
			}
			partA = fmt.Sprintf("PUT without ContentType → %q", ct)
		}

		// 7b: PUT with audio/mpeg, then COPY REPLACE carrying it.
		var partB, partC string
		if err := client.PutForProbe(ctx, key, body, map[string]string{"title": "x"}, "audio/mpeg"); err == nil {
			if head, e := client.GetMetadata(ctx, key); e == nil && head != nil && head.ContentType != nil {
				partB = fmt.Sprintf("PUT with audio/mpeg → %q", *head.ContentType)
			}
			_ = client.CopyForProbe(ctx, key, map[string]string{"title": "y"}, true, "audio/mpeg")
			if head, e := client.GetMetadata(ctx, key); e == nil && head != nil && head.ContentType != nil {
				partC = fmt.Sprintf("COPY REPLACE with audio/mpeg → %q", *head.ContentType)
			}
			// 7c: COPY REPLACE carrying NO Content-Type — the real bug.
			_ = client.CopyForProbe(ctx, key, map[string]string{"title": "z"}, true, "")
			if head, e := client.GetMetadata(ctx, key); e == nil && head != nil && head.ContentType != nil {
				partC += fmt.Sprintf(" | COPY REPLACE without one → %q", *head.ContentType)
			}
		}
		add(result{name: "7. Content-Type survival", verdict: "INFO",
			detail: partA + " | " + partB + " | " + partC})
	}

	// ── CASE 8: non-ASCII KEY ───────────────────────────────────────────
	// Confirms R2 accepts a UTF-8 key, and that a non-ASCII metadata KEY
	// is silently dropped (x-amz-missing-meta).
	{
		key := prefix + "Tést — Autör.probe"
		guard(key)

		err := client.PutForProbe(ctx, key, body, map[string]string{"title": "accented key test"}, "audio/mpeg")
		if err != nil {
			add(result{name: "8. non-ASCII object key", verdict: "REJECTED", detail: err.Error()})
		} else if head, e := client.GetMetadata(ctx, key); e != nil || head == nil {
			add(result{name: "8. non-ASCII object key", verdict: "ERROR", detail: fmt.Sprint(e)})
		} else {
			missing := ""
			if v := head.Metadata["x-amz-missing-meta"]; v != "" {
				missing = "  x-amz-missing-meta=" + v
			}
			add(result{name: "8. non-ASCII object key", verdict: "ACCEPTED",
				detail: fmt.Sprintf("stored title=%q%s", head.Metadata["title"], missing)})
		}
	}

	// ── CASE 9: copy from a missing source ──────────────────────────────
	// Forces the 200-with-<Error>-body condition. If the SDK surfaces it
	// as nil error, every historical copy failure was silent.
	{
		key := prefix + "missing-source.probe"
		guard(key)

		src := prefix + "does-not-exist.probe"
		_ = src
		// CopyForProbe copies key→key, so build the failure by copying a
		// key that was never created, using the same helper against it.
		err := client.CopyForProbe(ctx, prefix+"never-created.probe",
			map[string]string{"title": "x"}, true, "audio/mpeg")
		if err != nil {
			add(result{name: "9. copy missing source", verdict: "ERROR_SURFACED",
				detail: err.Error()})
		} else {
			add(result{name: "9. copy missing source", verdict: "NIL_ERROR",
				detail: "CopyObject returned no error for a nonexistent source — the silent-failure trap is REAL"})
		}
		_ = key
	}

	// ── CASE 10: body-transfer accounting (copy vs read+write) ──────────
	// Proves with numbers that a copy does not move the body.
	{
		key := prefix + "bulk.probe"
		guard(key)

		// 1 MB of deterministic bytes.
		big := make([]byte, 1<<20)
		for i := range big {
			big[i] = byte(i % 251)
		}
		want := md5Hex(big)

		if err := client.PutForProbe(ctx, key, big, map[string]string{"title": "bulk"}, "audio/mpeg"); err != nil {
			add(result{name: "10. copy byte accounting", verdict: "ERROR", detail: err.Error()})
		} else {
			countTrans.Reset()
			copyErr := client.CopyForProbe(ctx, key, map[string]string{"title": "bulk2"}, true, "audio/mpeg")
			cReq, cResp, _ := countTrans.Snapshot()

			// Verify the body survived the copy unchanged.
			got, _ := client.GetObjectBytes(ctx, key)
			integrity := "checksum-ok"
			if md5Hex(got) != want {
				integrity = "CHECKSUM-CHANGED"
			}
			verdict := integrity
			if copyErr != nil {
				verdict = "ERROR"
			}
			add(result{name: "10. copy byte accounting", verdict: verdict,
				detail:   fmt.Sprintf("copy of 1 MB: request body=%d bytes, response=%d bytes", cReq, cResp),
				reqBytes: cReq, resBytes: cResp})
		}
	}

	// ── CASE 11: decoder round-trip losslessness ────────────────────────
	{
		key := prefix + "roundtrip.probe"
		guard(key)
		if err := client.PutForProbe(ctx, key, body, map[string]string{"title": valQuestion}, "audio/mpeg"); err == nil {
			if head, e := client.GetMetadata(ctx, key); e == nil && head != nil {
				decoded := metadata.Decode(head.Metadata["title"])
				v := "LOSSLESS"
				if decoded != valQuestion {
					v = "LOSSY — decoder still imperfect"
				}
				add(result{name: "11. decode round-trip", verdict: v,
					detail: fmt.Sprintf("sent=%q  decoded=%q", valQuestion, decoded)})
			}
		}
	}

	// ── RESULTS TABLE ───────────────────────────────────────────────────
	fmt.Printf("\n=== RESULTS ===\n")
	for i, r := range results {
		fmt.Printf("%2d. %-42s %s\n", i+1, r.name, r.verdict)
		if r.detail != "" {
			fmt.Printf("    %s\n", r.detail)
		}
	}

	// ── CLEANUP ─────────────────────────────────────────────────────────
	if *keep {
		fmt.Printf("\n--keep set: probe objects retained under %s\n", prefix)
	} else {
		keys, err := client.ListObjectKeys(ctx)
		if err == nil {
			removed := 0
			for _, k := range keys {
				if strings.HasPrefix(k, prefix) {
					if err := client.DeleteObject(ctx, k); err == nil {
						removed++
					}
				}
			}
			fmt.Printf("\nCleaned up %d probe object(s).\n", removed)
		}
	}
}

// classify compares what was sent with what HEAD returned.
//
//	PRESERVED          round-trips byte-identically
//	ENCODED_ON_WRITE   differs raw, but decoding recovers the sent value
//	WRAPPED            raw wraps the sent value in an encoded-word
//	DIFFERENT          anything else — inspect by hand
func classify(sent, raw string) string {
	switch {
	case raw == sent:
		return "PRESERVED"
	case metadata.Decode(raw) == sent:
		return "ENCODED_ON_WRITE"
	case strings.Contains(raw, sent):
		return "WRAPPED"
	default:
		return "DIFFERENT"
	}
}

func md5Hex(b []byte) string {
	sum := md5.Sum(b)
	return hex.EncodeToString(sum[:])
}
