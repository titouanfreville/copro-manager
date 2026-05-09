// Package push wraps the webpush-go library to ship Web Push messages
// with VAPID-signed authentication. The PrivateKey + PublicKey pair is
// loaded from YAML config; in production the values come from Secret
// Manager via the layered conf-file mechanism described in AGENTS.md.
package push

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

// Config carries the VAPID keypair + the contact email Apple/Mozilla
// require in the JWT subject claim.
type Config struct {
	PrivateKey string `yaml:"private_key"`
	PublicKey  string `yaml:"public_key"`
	// Subject is a `mailto:` URL the push services use to contact the
	// app owner if the keys cause delivery problems. Defaults to a
	// placeholder if unset.
	Subject string `yaml:"subject"`
	// TTLSeconds is how long the push service should hold the message
	// when the device is offline. 24 hours by default.
	TTLSeconds int `yaml:"ttl_seconds"`
}

// Sender implements interfaces.PushSender via webpush-go.
type Sender struct {
	cfg Config
}

// NewSender returns a PushSender. Always returns a non-nil sender even
// when keys are empty — Send returns a clear error in that case so
// alerts.Fire can no-op gracefully without the rest of the app caring.
func NewSender(cfg Config) *Sender {
	if cfg.Subject == "" {
		cfg.Subject = "mailto:dev@example.invalid"
	}
	if cfg.TTLSeconds <= 0 {
		cfg.TTLSeconds = 24 * 60 * 60
	}
	return &Sender{cfg: cfg}
}

// Send dispatches a single push. Returns gone=true when the push service
// reports the subscription is permanently dead (HTTP 410 / 404) so the
// caller can prune the stale row.
func (s *Sender) Send(ctx context.Context, sub entities.PushSubscription, payload interfaces.PushPayload) (bool, error) {
	if s.cfg.PrivateKey == "" || s.cfg.PublicKey == "" {
		return false, fmt.Errorf("push: VAPID keys not configured")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return false, fmt.Errorf("push: marshal payload: %w", err)
	}
	wpSub := &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys: webpush.Keys{
			P256dh: sub.P256dh,
			Auth:   sub.Auth,
		},
	}
	opts := &webpush.Options{
		Subscriber:      s.cfg.Subject,
		VAPIDPublicKey:  s.cfg.PublicKey,
		VAPIDPrivateKey: s.cfg.PrivateKey,
		TTL:             s.cfg.TTLSeconds,
	}
	// Per-call short timeout so a hung push service can't stall an
	// alert-fire loop.
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	resp, err := webpush.SendNotificationWithContext(cctx, body, wpSub, opts)
	if err != nil {
		return false, fmt.Errorf("push: send: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound {
		return true, nil
	}
	if resp.StatusCode >= 400 {
		return false, fmt.Errorf("push: send failed (%d %s)", resp.StatusCode, strings.TrimSpace(resp.Status))
	}
	return false, nil
}

// Compile-time check that Sender satisfies the domain interface.
var _ interfaces.PushSender = (*Sender)(nil)
