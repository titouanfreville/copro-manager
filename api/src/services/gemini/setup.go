// Package gemini wraps Vertex AI Gemini for the copro-manager
// vision-tier features (meter OCR; document classifier and chat
// assistant land later as additional methods on the same Client).
//
// The wrapper is intentionally thin — `services/gemini` provides
// auth + per-month usage gating; each consumer (meters, documents,
// future chat) writes its own narrow method on top, mirroring the
// pattern used by `services/firestore` and `services/storage`.
package gemini

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/genai"

	"github.com/titouanfreville/copro-manager/api/src/domain/errors"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

// DefaultModel is the model used when Config.Model is empty. 2.5 Flash
// strikes the right accuracy/cost balance for visual reasoning at the
// 2-foyer scale (~€0.04/mo at 50 calls/month).
const DefaultModel = "gemini-2.5-flash"

// Config gates Gemini usage at the application boundary (NFR31).
// Enabled=false short-circuits every call with ErrFeatureDisabled so
// no Vertex AI charges accrue. MonthlyCallCap bounds calls per
// calendar month — once reached, the client returns ErrFeatureCapped
// until the bucket rolls over. Counter is persisted by AIUsageStore.
type Config struct {
	Enabled        bool   `yaml:"enabled"`
	MonthlyCallCap int64  `yaml:"monthly_call_cap"`
	ProjectID      string `yaml:"project_id"`
	Region         string `yaml:"region"`
	Model          string `yaml:"model"`
}

// Client wraps a genai.Client configured for Vertex AI. The single
// instance is shared by every consumer (currently only the meter
// reader; document analyzer and chat assistant slot in later).
type Client struct {
	c     *genai.Client
	cfg   Config
	usage interfaces.AIUsageStore
}

// NewClient builds the Vertex AI Gemini client. When cfg.Enabled is
// false the underlying genai.Client is NOT constructed — every public
// method returns ErrFeatureDisabled, so the project doesn't need
// Vertex AI provisioned for off-state.
//
// `usage` is required when cfg.Enabled is true so the cap is
// enforceable; pass nil to short-circuit gating in tests.
func NewClient(cfg Config, usage interfaces.AIUsageStore) (*Client, error) {
	// Default the model regardless of Enabled so test paths that flip
	// Enabled on the returned struct still see a usable Model string.
	if cfg.Model == "" {
		cfg.Model = DefaultModel
	}
	if !cfg.Enabled {
		return &Client{cfg: cfg, usage: usage}, nil
	}
	if cfg.ProjectID == "" || cfg.Region == "" {
		return nil, fmt.Errorf("gemini: project_id and region are required when enabled")
	}
	c, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		Backend:  genai.BackendVertexAI,
		Project:  cfg.ProjectID,
		Location: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("gemini: new client: %w", err)
	}
	return &Client{c: c, cfg: cfg, usage: usage}, nil
}

// Close is a no-op — the genai SDK manages its own HTTP client.
// Reserved for future SDK revisions and FX lifecycle symmetry with
// other GCP service wrappers.
func (c *Client) Close() error { return nil }

// monthKey returns the YYYY-MM bucket used to scope the counter.
// UTC is fine here — the cap is a budget, not a calendar invariant.
func monthKey(now time.Time) string {
	return now.UTC().Format("2006-01")
}

// gate checks the feature flag + monthly cap before a call. Returns
// ErrFeatureDisabled or ErrFeatureCapped as appropriate; nil when the
// caller may proceed and increment afterwards.
func (c *Client) gate(ctx context.Context) error {
	if !c.cfg.Enabled || c.c == nil {
		return errors.ErrFeatureDisabled
	}
	if c.cfg.MonthlyCallCap <= 0 || c.usage == nil {
		return nil
	}
	count, err := c.usage.CountForPeriod(ctx, monthKey(time.Now()))
	if err != nil {
		return fmt.Errorf("gemini: usage count: %w", err)
	}
	if count >= c.cfg.MonthlyCallCap {
		return errors.ErrFeatureCapped
	}
	return nil
}

// recordCall increments the per-month counter best-effort — if the
// increment fails the call already happened, so silent failure plus
// the next call discovering the drift is the responsible default.
func (c *Client) recordCall(ctx context.Context) {
	if c.usage == nil || c.cfg.MonthlyCallCap <= 0 {
		return
	}
	_ = c.usage.IncrementForPeriod(ctx, monthKey(time.Now()))
}
