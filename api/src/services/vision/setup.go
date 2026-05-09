// Package vision wraps the Google Cloud Vision API as an
// `interfaces.OCRService`. We only use DOCUMENT_TEXT_DETECTION (better
// suited to dense text like a meter dial than the loose TEXT_DETECTION
// path) and accept GCS URIs directly so meter photos stay in GCS — no
// egress through Cloud Run for the OCR round-trip.
package vision

import (
	"context"
	"fmt"
	"time"

	vision "cloud.google.com/go/vision/v2/apiv1"
	visionpb "cloud.google.com/go/vision/v2/apiv1/visionpb"

	domainerrors "github.com/titouanfreville/copro-manager/api/src/domain/errors"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

// Config gates Vision OCR usage at the application boundary (NFR31).
// `Enabled: false` short-circuits every call with ErrFeatureDisabled so
// no Vision charges accrue. `MonthlyCallCap` bounds the number of Vision
// calls per calendar month — once exceeded, the client returns
// ErrFeatureCapped until the month rolls over. Counter is persisted by
// `interfaces.VisionUsageStore`.
type Config struct {
	Enabled        bool  `yaml:"enabled"`
	MonthlyCallCap int64 `yaml:"monthly_call_cap"`
}

// Client wraps a Vision API ImageAnnotator client.
type Client struct {
	c     *vision.ImageAnnotatorClient
	cfg   Config
	usage interfaces.VisionUsageStore
}

// NewClient creates a Vision client. The API must be enabled in the
// project (`vision.googleapis.com`). Authentication is via Application
// Default Credentials — Cloud Run's runtime SA has implicit access
// once the API is enabled.
//
// `usage` is the per-month counter store; mandatory when cfg.Enabled is
// true so the cap is enforceable. Pass nil to short-circuit gating
// (only useful for tests / off-state).
func NewClient(cfg Config, usage interfaces.VisionUsageStore) (*Client, error) {
	c, err := vision.NewImageAnnotatorClient(context.Background())
	if err != nil {
		return nil, fmt.Errorf("vision: new client: %w", err)
	}
	return &Client{c: c, cfg: cfg, usage: usage}, nil
}

// Close releases the underlying client.
func (c *Client) Close() error { return c.c.Close() }

// monthKey returns the YYYY-MM bucket used to scope the per-month
// counter. Europe/Paris is acceptable here — the cap is a budget, not
// a calendar invariant.
func monthKey(now time.Time) string {
	return now.UTC().Format("2006-01")
}

// gate checks the feature flag + monthly cap before a call. Returns
// ErrFeatureDisabled or ErrFeatureCapped as appropriate; nil when the
// caller may proceed and increment afterwards.
func (c *Client) gate(ctx context.Context) error {
	if !c.cfg.Enabled {
		return domainerrors.ErrFeatureDisabled
	}
	if c.cfg.MonthlyCallCap <= 0 || c.usage == nil {
		// No cap configured (or tests passed nil) — allow.
		return nil
	}
	count, err := c.usage.CountForPeriod(ctx, monthKey(time.Now()))
	if err != nil {
		return fmt.Errorf("vision: usage count: %w", err)
	}
	if count >= c.cfg.MonthlyCallCap {
		return domainerrors.ErrFeatureCapped
	}
	return nil
}

// recordCall increments the per-month counter. Best-effort — if the
// increment fails the call already happened so silently log-and-move-on
// is the responsible default; the next call will discover the drift.
func (c *Client) recordCall(ctx context.Context) {
	if c.usage == nil || c.cfg.MonthlyCallCap <= 0 {
		return
	}
	_ = c.usage.IncrementForPeriod(ctx, monthKey(time.Now()))
}

// DetectText runs DOCUMENT_TEXT_DETECTION against the given GCS URI
// (gs://bucket/path/to/object). Returns one OCRTextBlock per detected
// word, with normalized bounding-box coordinates.
//
// The GCS URI variant means Vision pulls the bytes itself; we don't
// stream through Cloud Run. Vision accepts our private bucket because
// the runtime SA has read access to it (granted at deploy time).
func (c *Client) DetectText(ctx context.Context, gcsURI string) ([]interfaces.OCRTextBlock, error) {
	if gcsURI == "" {
		return nil, fmt.Errorf("vision: empty gcs uri")
	}
	if err := c.gate(ctx); err != nil {
		return nil, err
	}
	out, err := c.detect(ctx, &visionpb.Image{
		Source: &visionpb.ImageSource{ImageUri: gcsURI},
	}, gcsURI)
	if err == nil {
		c.recordCall(ctx)
	}
	return out, err
}

// DetectTextFromBytes runs DOCUMENT_TEXT_DETECTION against an inline
// image payload. Used by the capture flow's "Auto-lire" button where
// the photo hasn't yet been written to GCS.
func (c *Client) DetectTextFromBytes(ctx context.Context, image []byte) ([]interfaces.OCRTextBlock, error) {
	if len(image) == 0 {
		return nil, fmt.Errorf("vision: empty image bytes")
	}
	if err := c.gate(ctx); err != nil {
		return nil, err
	}
	out, err := c.detect(ctx, &visionpb.Image{Content: image}, "<inline>")
	if err == nil {
		c.recordCall(ctx)
	}
	return out, err
}

func (c *Client) detect(ctx context.Context, img *visionpb.Image, label string) ([]interfaces.OCRTextBlock, error) {
	batch, err := c.c.BatchAnnotateImages(ctx, &visionpb.BatchAnnotateImagesRequest{
		Requests: []*visionpb.AnnotateImageRequest{
			{
				Image: img,
				Features: []*visionpb.Feature{
					{Type: visionpb.Feature_DOCUMENT_TEXT_DETECTION, MaxResults: 1},
				},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("vision: annotate %s: %w", label, err)
	}
	if len(batch.GetResponses()) == 0 {
		return nil, nil
	}
	resp := batch.GetResponses()[0]
	if errResp := resp.GetError(); errResp != nil && errResp.GetCode() != 0 {
		return nil, fmt.Errorf("vision: annotate %s: code=%d %s", label, errResp.GetCode(), errResp.GetMessage())
	}
	full := resp.GetFullTextAnnotation()
	if full == nil {
		return nil, nil
	}

	// Walk page → block → paragraph → word and emit one OCRTextBlock
	// per word. Word granularity is the right level for meter dials —
	// each digit cluster typically lands as one word.
	var out []interfaces.OCRTextBlock
	for _, page := range full.GetPages() {
		pageW := float64(page.GetWidth())
		pageH := float64(page.GetHeight())
		if pageW <= 0 || pageH <= 0 {
			// Without page dimensions we can't normalize; emit everything
			// at (0,0,0,0) so the consumer can still use the text.
			pageW, pageH = 1, 1
		}
		for _, block := range page.GetBlocks() {
			for _, para := range block.GetParagraphs() {
				for _, word := range para.GetWords() {
					var sb []byte
					for _, sym := range word.GetSymbols() {
						sb = append(sb, sym.GetText()...)
					}
					if len(sb) == 0 {
						continue
					}
					box := word.GetBoundingBox()
					var minX, minY, maxX, maxY float64
					if box != nil && len(box.GetVertices()) > 0 {
						minX, minY = float64(box.GetVertices()[0].GetX()), float64(box.GetVertices()[0].GetY())
						maxX, maxY = minX, minY
						for _, v := range box.GetVertices() {
							x, y := float64(v.GetX()), float64(v.GetY())
							if x < minX {
								minX = x
							}
							if x > maxX {
								maxX = x
							}
							if y < minY {
								minY = y
							}
							if y > maxY {
								maxY = y
							}
						}
					}
					out = append(out, interfaces.OCRTextBlock{
						Text:       string(sb),
						Confidence: float64(word.GetConfidence()),
						X:          minX / pageW,
						Y:          minY / pageH,
						Width:      (maxX - minX) / pageW,
						Height:     (maxY - minY) / pageH,
					})
				}
			}
		}
	}
	return out, nil
}
