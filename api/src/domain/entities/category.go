package entities

// Category groups expenses (and later, documents). The seed list is bootstrapped
// lazily on first list request so a fresh deployment has the standard French
// copro categories without a separate seed step.
//
// Hidden categories are still available for FK reference (CSV importer triage
// fallback) but are filtered out of the user-facing list to match PRD FR10
// (which enumerates exactly six visible categories).
type Category struct {
	ID                      string           `json:"id"`
	Name                    string           `json:"name"`
	Predefined              bool             `json:"predefined"`
	Hidden                  bool             `json:"hidden,omitempty"`
	DefaultDistributionMode DistributionMode `json:"default_distribution_mode,omitempty"`
}

// PredefinedCategories is the minimum set bootstrapped on first List call.
// Order matches the PRD; IDs are stable so repeated calls converge on the
// same documents.
var PredefinedCategories = []Category{
	{ID: "eau", Name: "Eau", Predefined: true, DefaultDistributionMode: DistributionModeEqual},
	{ID: "electricite", Name: "Électricité", Predefined: true, DefaultDistributionMode: DistributionModeEqual},
	{ID: "taxe-fonciere", Name: "Taxe foncière", Predefined: true, DefaultDistributionMode: DistributionModeTantiemes},
	{ID: "travaux", Name: "Travaux", Predefined: true, DefaultDistributionMode: DistributionModeEqual},
	{ID: "assurance", Name: "Assurance", Predefined: true, DefaultDistributionMode: DistributionModeTantiemes},
	{ID: "syndic", Name: "Syndic", Predefined: true, DefaultDistributionMode: DistributionModeTantiemes},
	// Catch-all for the CSV importer when an item label doesn't match any
	// of the keyword heuristics above. Hidden from the user-facing list —
	// the importer still references it by ID for triage rows.
	{ID: "tbd", Name: "À catégoriser", Predefined: true, Hidden: true},
}
