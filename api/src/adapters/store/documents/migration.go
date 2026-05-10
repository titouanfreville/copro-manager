package documents

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	fs "cloud.google.com/go/firestore"
	"go.uber.org/zap"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// expensesCollection mirrors the constant in adapters/expenses but stays
// local so this file compiles standalone (and so a future rename of
// either collection name is an explicit, file-spanning change).
const (
	expensesCollection        = "expenses"
	attachmentsSubcollection  = "attachments"
	migrationLoggerName       = "migrations.attachments_to_documents"
	migrationPerExpenseBudget = 30 * time.Second
)

// legacyAttachment is the on-disk shape of an attachment doc in the
// pre-migration subcollection `expenses/{id}/attachments/{aid}`.
type legacyAttachment struct {
	ID               string    `firestore:"id"`
	ObjectName       string    `firestore:"object_name"`
	ContentType      string    `firestore:"content_type"`
	SizeBytes        int64     `firestore:"size_bytes"`
	OriginalFilename string    `firestore:"original_filename"`
	UploadedAt       time.Time `firestore:"uploaded_at"`
	UploadedBy       string    `firestore:"uploaded_by"`
}

// expenseSlim grabs only the fields the migration needs from each expense
// doc. We intentionally do not import the expenses adapter's internal
// expenseDoc type — that would entangle the two adapters and force them
// to evolve in lockstep.
type expenseSlim struct {
	CategoryID string `firestore:"category_id"`
	Name       string `firestore:"name"`
	CoproID    string `firestore:"copro_id"`
}

// MigrationSummary is logged at the end of the run so an operator can
// confirm the migration touched what they expected.
type MigrationSummary struct {
	ExpensesScanned     int
	AttachmentsFound    int
	DocumentsCreated    int
	AlreadyMigrated     int
	LegacyDocsDeleted   int
	ExpenseLookupErrors int
	WriteErrors         int
}

// MigrateAttachmentsToDocuments collapses every legacy
// `expenses/{id}/attachments/{aid}` subdoc into a top-level
// `documents/{aid}` doc with `linked_expense_id` set. The GCS object name
// is preserved verbatim — no blob copy needed; the existing
// `expenses/{id}/{aid}{ext}` keys keep working through the Document's
// stored ObjectName.
//
// Idempotent: a Document already at the target ID is left alone (the
// legacy subdoc is still removed so the next boot doesn't re-process
// it). Best-effort: a per-attachment failure is logged and the loop
// keeps going — a single bad row should not strand the rest. Safe to
// re-run.
//
// Volume assumption: <100 attachments total at our 2-foyer scale.
// No batching, no concurrency, no resumable cursor — those would be
// premature complexity here.
func MigrateAttachmentsToDocuments(ctx context.Context, client *fs.Client, logger *zap.Logger) error {
	log := logger.Named(migrationLoggerName)
	summary := &MigrationSummary{}

	expIter := client.Collection(expensesCollection).Documents(ctx)
	defer expIter.Stop()

	for {
		expSnap, err := expIter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return fmt.Errorf("migration: list expenses: %w", err)
		}
		summary.ExpensesScanned++

		var exp expenseSlim
		if err := expSnap.DataTo(&exp); err != nil {
			summary.ExpenseLookupErrors++
			log.Warn("expense decode failed (skipping its attachments)",
				zap.String("expense_id", expSnap.Ref.ID),
				zap.Error(err))
			continue
		}

		if err := migrateOneExpense(ctx, client, log, expSnap.Ref.ID, exp, summary); err != nil {
			log.Warn("per-expense migration leg failed (continuing)",
				zap.String("expense_id", expSnap.Ref.ID),
				zap.Error(err))
		}
	}

	log.Info("Success",
		zap.Int("expenses_scanned", summary.ExpensesScanned),
		zap.Int("attachments_found", summary.AttachmentsFound),
		zap.Int("documents_created", summary.DocumentsCreated),
		zap.Int("already_migrated", summary.AlreadyMigrated),
		zap.Int("legacy_docs_deleted", summary.LegacyDocsDeleted),
		zap.Int("expense_lookup_errors", summary.ExpenseLookupErrors),
		zap.Int("write_errors", summary.WriteErrors),
	)
	return nil
}

func migrateOneExpense(
	ctx context.Context,
	client *fs.Client,
	log *zap.Logger,
	expenseID string,
	exp expenseSlim,
	summary *MigrationSummary,
) error {
	ctx, cancel := context.WithTimeout(ctx, migrationPerExpenseBudget)
	defer cancel()

	subRef := client.Collection(expensesCollection).Doc(expenseID).Collection(attachmentsSubcollection)
	attIter := subRef.Documents(ctx)
	defer attIter.Stop()

	for {
		attSnap, err := attIter.Next()
		if errors.Is(err, iterator.Done) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("list attachments: %w", err)
		}
		summary.AttachmentsFound++

		var legacy legacyAttachment
		if err := attSnap.DataTo(&legacy); err != nil {
			summary.WriteErrors++
			log.Warn("legacy attachment decode failed (leaving in place)",
				zap.String("expense_id", expenseID),
				zap.String("attachment_id", attSnap.Ref.ID),
				zap.Error(err))
			continue
		}
		// Defense-in-depth: if the doc lacks an explicit id field, fall
		// back to the snapshot's path id. Same for object_name (a record
		// without one would 404 on download anyway, but we still migrate
		// the metadata so a human can spot the orphan).
		if legacy.ID == "" {
			legacy.ID = attSnap.Ref.ID
		}

		// Idempotency check: if the target Document is already there,
		// don't overwrite it (we'd risk clobbering an in-progress edit).
		// Just clean up the legacy subdoc and move on.
		targetRef := client.Collection(collection).Doc(legacy.ID)
		_, err = targetRef.Get(ctx)
		switch {
		case err == nil:
			summary.AlreadyMigrated++
			if _, delErr := attSnap.Ref.Delete(ctx); delErr != nil {
				log.Warn("legacy delete failed (already-migrated row)",
					zap.String("attachment_id", legacy.ID), zap.Error(delErr))
			} else {
				summary.LegacyDocsDeleted++
			}
			continue
		case status.Code(err) == codes.NotFound:
			// expected — fall through to migrate
		default:
			summary.WriteErrors++
			log.Warn("target lookup failed (leaving legacy in place)",
				zap.String("attachment_id", legacy.ID), zap.Error(err))
			continue
		}

		newDoc := documentDoc{
			ID:               legacy.ID,
			CoproID:          exp.CoproID,
			CategoryID:       exp.CategoryID,
			Title:            deriveMigrationTitle(legacy.OriginalFilename, exp.Name),
			ObjectName:       legacy.ObjectName,
			ContentType:      legacy.ContentType,
			SizeBytes:        legacy.SizeBytes,
			OriginalFilename: legacy.OriginalFilename,
			UploadedAt:       legacy.UploadedAt,
			UploadedBy:       legacy.UploadedBy,
			LinkedExpenseID:  expenseID,
		}
		if _, err := targetRef.Create(ctx, newDoc); err != nil {
			summary.WriteErrors++
			log.Warn("target create failed (leaving legacy in place)",
				zap.String("attachment_id", legacy.ID), zap.Error(err))
			continue
		}
		summary.DocumentsCreated++

		// Drop the legacy row only after the target write succeeded —
		// otherwise a crash mid-migration could lose data.
		if _, err := attSnap.Ref.Delete(ctx); err != nil {
			log.Warn("legacy delete failed (target was created — orphan subdoc remains)",
				zap.String("attachment_id", legacy.ID), zap.Error(err))
			continue
		}
		summary.LegacyDocsDeleted++
	}
}

func deriveMigrationTitle(filename, expenseName string) string {
	t := strings.TrimSpace(filename)
	if t != "" {
		return t
	}
	t = strings.TrimSpace(expenseName)
	if t != "" {
		return t
	}
	return "Pièce jointe"
}
