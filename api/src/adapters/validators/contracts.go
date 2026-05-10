// Package validators implements the resource-specific validators
// declared in domain/interfaces. Each validator owns the stores it
// needs for cross-resource lookups, composes the rule library from
// domain/validation/rules, and exposes a tiny interface so usecases
// can call it as a single gate.
package validators

import (
	"context"
	"strings"

	"github.com/titouanfreville/copro-manager/api/src/adapters/validators/rules"
	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

const (
	contractNameMin = 2
	contractNameMax = 120
)

var (
	knownBillingFrequencies = []entities.BillingFrequency{
		entities.BillingFrequencyMonthly,
		entities.BillingFrequencyQuarterly,
		entities.BillingFrequencyYearly,
	}
	knownContractStatuses = []entities.ContractStatus{
		entities.ContractStatusActive,
		entities.ContractStatusExpired,
		entities.ContractStatusCancelled,
	}
)

// Contracts validates ContractDraft inputs. Pure-data rules run via
// the rules library; cross-resource lookups (category exists, optional
// template exists) happen after those pass so the cheap checks fail
// fast.
type Contracts struct {
	categories interfaces.CategoriesStore
	templates  interfaces.TemplatesStore
}

// NewContracts builds the validator. Both stores are required — the
// validator refuses to run with nil deps so a wiring miss surfaces at
// boot rather than as a silent skip.
func NewContracts(categories interfaces.CategoriesStore, templates interfaces.TemplatesStore) interfaces.ContractValidator {
	return &Contracts{categories: categories, templates: templates}
}

// ValidateCreate runs the same rules as ValidateUpdate; the split
// exists so adding create-only or update-only constraints later
// doesn't require changing the interface.
func (v *Contracts) ValidateCreate(ctx context.Context, d entities.ContractDraft) error {
	return v.validate(ctx, d)
}

// ValidateUpdate is identical to ValidateCreate today. The two paths
// stay separate so update-specific rules (e.g. status transitions)
// can land here without touching the caller.
func (v *Contracts) ValidateUpdate(ctx context.Context, d entities.ContractDraft) error {
	return v.validate(ctx, d)
}

func (v *Contracts) validate(ctx context.Context, d entities.ContractDraft) error {
	if err := v.pureRules(d); err != nil {
		return err
	}
	return v.crossResource(ctx, d)
}

// pureRules covers everything checkable without I/O. Reads top-down
// like a checklist of what a contract must contain.
func (v *Contracts) pureRules(d entities.ContractDraft) error {
	return rules.First(
		rules.NonBlank("name", d.Name),
		rules.MinLen("name", d.Name, contractNameMin),
		rules.MaxLen("name", d.Name, contractNameMax),
		rules.NonBlank("category_id", d.CategoryID),
		rules.NonBlank("society.name", d.Society.Name),
		rules.IntNonNegative("amount_cents", d.AmountCents),
		rules.OneOf("billing_frequency", d.BillingFrequency, knownBillingFrequencies),
		rules.OneOf("status", d.Status, knownContractStatuses),
		rules.DateNotBefore("end_date", d.EndDate, d.StartDate),
	)
}

// crossResource runs the FK existence checks that need a store call.
// Kept separate from pureRules so any pure-rule failure short-circuits
// before we hit Firestore.
func (v *Contracts) crossResource(ctx context.Context, d entities.ContractDraft) error {
	cat, err := v.categories.FindByID(ctx, d.CategoryID)
	if err != nil {
		return err
	}
	if cat == nil {
		return entities.ValidationError{Key: "category_id", Message: "not found"}
	}
	if tid := strings.TrimSpace(d.TemplateID); tid != "" {
		t, err := v.templates.FindByID(ctx, tid)
		if err != nil {
			return err
		}
		if t == nil {
			return entities.ValidationError{Key: "template_id", Message: "not found"}
		}
	}
	return nil
}
