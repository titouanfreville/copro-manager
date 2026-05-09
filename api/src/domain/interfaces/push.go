package interfaces

import (
	"context"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

// PushSubscriptionsStore persists per-device push subscriptions.
type PushSubscriptionsStore interface {
	// Upsert writes the subscription, replacing any existing row keyed by
	// the same Endpoint (same browser re-subscribing).
	Upsert(ctx context.Context, s entities.PushSubscription) error

	// FindByEndpoint returns the subscription stored for the given
	// endpoint URL, or (nil, nil) when none exists. Used to enforce
	// ownership on Unsubscribe and to detect cross-foyer endpoint
	// takeover.
	FindByEndpoint(ctx context.Context, endpoint string) (*entities.PushSubscription, error)

	// DeleteByEndpoint removes the subscription by its endpoint URL.
	// Idempotent: missing rows are no-ops.
	DeleteByEndpoint(ctx context.Context, endpoint string) error

	// ListByFoyer returns every subscription belonging to the given
	// foyer — used to fan out a push send.
	ListByFoyer(ctx context.Context, foyerID string) ([]entities.PushSubscription, error)
}

// PushPayload is what we send to a browser. Title + body show in the
// system toast; deep-link is wired into the SW notificationclick handler.
type PushPayload struct {
	Title    string `json:"title"`
	Body     string `json:"body"`
	DeepLink string `json:"deep_link,omitempty"`
	AlertID  string `json:"alert_id,omitempty"`
}

// PushSender ships a payload to a single subscription. 410 Gone (subscription
// expired/revoked) is conveyed via the `gone` return value so the caller can
// auto-prune the stale row.
type PushSender interface {
	Send(ctx context.Context, sub entities.PushSubscription, payload PushPayload) (gone bool, err error)
}
