// Package rules hosts the validation framework: a tiny `Rule` type
// (deferred error thunk) plus the primitive, composable checks every
// resource validator pulls from. Resource-specific validators
// (adapters/validators/contracts.go etc.) wire these into the
// interface declared in domain/interfaces — domain stays free of
// validation tooling, only the contract.
//
// Reading a usecase shouldn't require knowing what "min length 2"
// looks like in code — it should call `validator.ValidateCreate(ctx,
// draft)` and move on.
package rules

// Rule is the single primitive every check returns. It's a function
// so pure CPU rules and lazy I/O rules share the same combinator.
// Each rule is responsible for tagging the failing field on the
// returned entities.ValidationError.
type Rule func() error

// First runs every rule in order and returns the first non-nil
// error, or nil when all rules pass. Designed for fail-fast input
// validation: the user only sees one error at a time, the cheapest
// checks run first.
func First(rules ...Rule) error {
	for _, r := range rules {
		if r == nil {
			continue
		}
		if err := r(); err != nil {
			return err
		}
	}
	return nil
}
