package entities

// DefaultTotalParts is the standard French copro tantième total when no other
// scheme is in use. Most building deeds (règlement de copropriété) express
// shares out of 1000.
const DefaultTotalParts = 1000

// Copro represents the property under copropriété — the building itself.
// Exactly one Copro instance exists for the lifetime of this application;
// the entity exists to give name, address, and TotalParts a stable home.
//
// Detail editing (name, address, total_parts) is intentionally out of MVP
// scope — the Copro is seeded with sensible defaults and edited via direct
// data fix until a future story exposes a settings UI.
type Copro struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Address    string `json:"address"`
	TotalParts int    `json:"total_parts"`
}
