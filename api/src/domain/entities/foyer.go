package entities

// FoyerFloor is a human-readable tag identifying which physical floor of the
// building a foyer occupies. It is a property of the foyer, not its identity:
// the canonical ID of a Foyer is an opaque UUID stored in Foyer.ID.
type FoyerFloor string

const (
	FoyerFloorRDC FoyerFloor = "rdc"
	FoyerFloor1er FoyerFloor = "1er"
)

// Foyer represents a household participating in the copropriété.
//
// Ownership is expressed in tantièmes: Foyer.Parts out of Copro.TotalParts.
// The sum of all foyers' Parts under a given CoproID must equal that copro's
// TotalParts; this invariant is enforced on edit, not by the type system.
//
// MemberIDs holds the User.ID of every person attached to this foyer. The
// authoritative person record lives in the users collection — Firebase Auth
// metadata is reachable via User.FirebaseUID.
type Foyer struct {
	ID        string     `json:"id"`
	CoproID   string     `json:"copro_id"`
	Floor     FoyerFloor `json:"floor"`
	Name      string     `json:"name"`
	Parts     int        `json:"parts"`
	MemberIDs []string   `json:"member_ids"`
}

// AllFoyerFloors lists the canonical set of foyer floors in display order.
func AllFoyerFloors() []FoyerFloor {
	return []FoyerFloor{FoyerFloorRDC, FoyerFloor1er}
}
