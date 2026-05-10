package copros

import (
	"context"
	"errors"
	"fmt"
	"sort"

	fs "cloud.google.com/go/firestore"
	"go.uber.org/zap"
	"google.golang.org/api/iterator"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	domainerrors "github.com/titouanfreville/copro-manager/api/src/domain/errors"
)

// rewriteTargets enumerates every collection that stores a `copro_id`
// foreign key. Each entry maps a Firestore collection name to the
// per-doc `copro_id` field path so the consolidation loop stays
// declarative and easy to extend.
var rewriteTargets = []rewriteTarget{
	{collection: "expenses", field: "copro_id"},
	{collection: "settlements", field: "copro_id"},
	{collection: "documents", field: "copro_id"},
	{collection: "expense_templates", field: "copro_id"},
	{collection: "meter_readings", field: "copro_id"},
	{collection: "alerts", field: "copro_id"},
}

const foyersCollection = "foyers"

type rewriteTarget struct {
	collection string
	field      string
}

// ConsolidationSummary is what the admin route returns to the operator
// after the merge finishes. Per-collection counts let the user verify at
// a glance that the rewrites landed where they expected.
type ConsolidationSummary struct {
	CanonicalCoproID string         `json:"canonical_copro_id"`
	Rewritten        map[string]int `json:"rewritten"`
	DeletedCoproIDs  []string       `json:"deleted_copro_ids"`
	DryRun           bool           `json:"dry_run"`
}

// ConsolidationOptions tunes the merge. CanonicalCoproIDOverride lets
// the caller force a specific Copro id when the auto-detect path
// (foyer consensus) fails. DryRun reports what would change without
// writing anything.
type ConsolidationOptions struct {
	CanonicalCoproIDOverride string
	DryRun                   bool
}

// Consolidate picks the canonical Copro doc, rewrites every dependent
// row's `copro_id` to point at it, and deletes orphan Copro docs.
// Idempotent: running it on a clean slate (one Copro, every foreign key
// already pointing at it) is a no-op that returns zero rewrites.
//
// Canonical resolution order:
//  1. CanonicalCoproIDOverride if non-empty AND the doc exists.
//  2. The Copro id every foyer's `copro_id` agrees on.
//  3. Otherwise: error — the operator must pass an override.
//
// Called from the admin route POST /admin/copros/consolidate. Not
// invoked at boot — this is a one-shot data-fix tool, not a migration.
func Consolidate(ctx context.Context, client *fs.Client, logger *zap.Logger, opts ConsolidationOptions) (*ConsolidationSummary, error) {
	log := logger.Named("ops.consolidate_copros")

	canonical, err := resolveCanonicalCopro(ctx, client, opts.CanonicalCoproIDOverride)
	if err != nil {
		return nil, err
	}
	log.Info("canonical resolved", zap.String("canonical_copro_id", canonical))

	allCopros, err := listAllCoproIDs(ctx, client)
	if err != nil {
		return nil, err
	}
	if !contains(allCopros, canonical) {
		return nil, fmt.Errorf("%w: canonical copro %q not found in collection", domainerrors.ErrNotFound, canonical)
	}

	summary := &ConsolidationSummary{
		CanonicalCoproID: canonical,
		Rewritten:        map[string]int{},
		DeletedCoproIDs:  []string{},
		DryRun:           opts.DryRun,
	}

	// Rewrite copro_id on every dependent row that points at any
	// non-canonical Copro. Iterating via an `in` filter wouldn't fit
	// our < 30 row dataset advantage; we run one query per orphan id
	// per collection. Cheap.
	orphans := []string{}
	for _, id := range allCopros {
		if id == canonical {
			continue
		}
		orphans = append(orphans, id)
	}

	for _, target := range rewriteTargets {
		count := 0
		for _, orphan := range orphans {
			n, err := rewriteCoproIDIn(ctx, client, target, orphan, canonical, opts.DryRun)
			if err != nil {
				return nil, fmt.Errorf("rewrite %s for %q: %w", target.collection, orphan, err)
			}
			count += n
		}
		if count > 0 {
			summary.Rewritten[target.collection] = count
			log.Info("collection rewritten",
				zap.String("collection", target.collection),
				zap.Int("rows", count))
		}
	}

	if !opts.DryRun {
		for _, orphan := range orphans {
			if _, err := client.Collection(collection).Doc(orphan).Delete(ctx); err != nil {
				return nil, fmt.Errorf("delete orphan copro %q: %w", orphan, err)
			}
			summary.DeletedCoproIDs = append(summary.DeletedCoproIDs, orphan)
			log.Info("orphan copro deleted", zap.String("copro_id", orphan))
		}
	} else {
		summary.DeletedCoproIDs = append(summary.DeletedCoproIDs, orphans...)
	}

	return summary, nil
}

// resolveCanonicalCopro picks the Copro id to keep. Override wins when
// supplied; otherwise the foyer-consensus is used.
func resolveCanonicalCopro(ctx context.Context, client *fs.Client, override string) (string, error) {
	if override != "" {
		ref := client.Collection(collection).Doc(override)
		_, err := ref.Get(ctx)
		if err != nil {
			return "", fmt.Errorf("override copro lookup: %w", err)
		}
		return override, nil
	}

	foyerCoproIDs, err := listFoyerCoproIDs(ctx, client)
	if err != nil {
		return "", err
	}
	if len(foyerCoproIDs) == 0 {
		return "", entities.ValidationError{
			Key:     "canonical_copro_id",
			Message: "no foyers exist — pass canonical_copro_id explicitly",
		}
	}
	uniq := uniqueValues(foyerCoproIDs)
	if len(uniq) == 1 && uniq[0] != "" {
		return uniq[0], nil
	}
	return "", entities.ValidationError{
		Key:     "canonical_copro_id",
		Message: fmt.Sprintf("foyers reference %d distinct copro ids (%v) — pass canonical_copro_id explicitly", len(uniq), uniq),
	}
}

// listFoyerCoproIDs returns the copro_id field of every foyer. Empty
// strings are included so the caller can detect the "foyers have no
// copro" anomaly.
func listFoyerCoproIDs(ctx context.Context, client *fs.Client) ([]string, error) {
	iter := client.Collection(foyersCollection).Documents(ctx)
	defer iter.Stop()

	var out []string
	for {
		snap, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("list foyers: %w", err)
		}
		var raw struct {
			CoproID string `firestore:"copro_id"`
		}
		if err := snap.DataTo(&raw); err != nil {
			return nil, fmt.Errorf("decode foyer: %w", err)
		}
		out = append(out, raw.CoproID)
	}
}

func listAllCoproIDs(ctx context.Context, client *fs.Client) ([]string, error) {
	iter := client.Collection(collection).Documents(ctx)
	defer iter.Stop()

	var out []string
	for {
		snap, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			sort.Strings(out)
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("list copros: %w", err)
		}
		out = append(out, snap.Ref.ID)
	}
}

// rewriteCoproIDIn updates every doc in the target collection whose
// `<field>` equals `from` so it points at `to`. Returns the number of
// rows actually rewritten. DryRun counts matches without writing.
func rewriteCoproIDIn(ctx context.Context, client *fs.Client, target rewriteTarget, from, to string, dryRun bool) (int, error) {
	iter := client.Collection(target.collection).Where(target.field, "==", from).Documents(ctx)
	defer iter.Stop()

	count := 0
	for {
		snap, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			return count, nil
		}
		if err != nil {
			return count, fmt.Errorf("iterate %s: %w", target.collection, err)
		}
		count++
		if dryRun {
			continue
		}
		if _, err := snap.Ref.Update(ctx, []fs.Update{{Path: target.field, Value: to}}); err != nil {
			return count, fmt.Errorf("update %s/%s: %w", target.collection, snap.Ref.ID, err)
		}
	}
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func uniqueValues(in []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
