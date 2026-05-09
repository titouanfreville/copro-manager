package entities

import "time"

// PushSubscription is one device + browser combination registered to
// receive Web Push notifications. Foyer-keyed: when an alert fires for
// a foyer, every subscription belonging to that foyer is fanned out.
//
// `Endpoint` is the unique identifier the browser provides; we use it as
// the upsert key (same browser re-subscribing replaces the prior row).
type PushSubscription struct {
	ID        string    `json:"id"`
	FoyerID   string    `json:"foyer_id"`
	Endpoint  string    `json:"endpoint"`
	P256dh    string    `json:"p256dh"`
	Auth      string    `json:"auth"`
	UserAgent string    `json:"user_agent,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
