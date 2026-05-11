// Package documents persists standalone Document entities in Firestore.
package documents

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	fs "cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	domainerrorsPkg "github.com/titouanfreville/copro-manager/api/src/domain/errors"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

const collection = "documents"

type documentDoc struct {
	ID               string            `firestore:"id"`
	CoproID          string            `firestore:"copro_id"`
	CategoryID       string            `firestore:"category_id"`
	Group            string            `firestore:"group,omitempty"`
	Title            string            `firestore:"title"`
	Description      string            `firestore:"description,omitempty"`
	ObjectName       string            `firestore:"object_name"`
	ContentType      string            `firestore:"content_type"`
	SizeBytes        int64             `firestore:"size_bytes"`
	OriginalFilename string            `firestore:"original_filename"`
	UploadedAt       time.Time         `firestore:"uploaded_at"`
	UploadedBy       string            `firestore:"uploaded_by"`
	LinkedExpenseID  string            `firestore:"linked_expense_id,omitempty"`
	LinkedContractID string            `firestore:"linked_contract_id,omitempty"`
	Analysis         *documentAnalysis `firestore:"analysis,omitempty"`
}

// documentAnalysis is the firestore-side mirror of entities.DocumentAnalysis
// (kept here per the layering rule: firestore tags never leak into the
// domain entity). Nested map shape — both extraction sub-objects are
// pointer-typed so absent ones round-trip as nil rather than empty.
type documentAnalysis struct {
	Kind       string              `firestore:"kind"`
	Confidence float64             `firestore:"confidence"`
	AnalyzedAt time.Time           `firestore:"analyzed_at"`
	Model      string              `firestore:"model,omitempty"`
	Reason     string              `firestore:"reason,omitempty"`
	Expense    *expenseExtraction  `firestore:"expense,omitempty"`
	Contract   *contractExtraction `firestore:"contract,omitempty"`
}

type expenseExtraction struct {
	AmountEUR    float64 `firestore:"amount_eur,omitempty"`
	Date         string  `firestore:"date,omitempty"`
	Vendor       string  `firestore:"vendor,omitempty"`
	CategoryHint string  `firestore:"category_hint,omitempty"`
	Description  string  `firestore:"description,omitempty"`
}

type contractExtraction struct {
	Provider         string  `firestore:"provider,omitempty"`
	ContractType     string  `firestore:"contract_type,omitempty"`
	StartDate        string  `firestore:"start_date,omitempty"`
	EndDate          string  `firestore:"end_date,omitempty"`
	MonthlyAmountEUR float64 `firestore:"monthly_amount_eur,omitempty"`
	ContractNumber   string  `firestore:"contract_number,omitempty"`
}

type Store struct {
	client *fs.Client
}

// NewStore returns a Firestore-backed documents store.
func NewStore(client *fs.Client) interfaces.DocumentsStore {
	return &Store{client: client}
}

func (s *Store) List(ctx context.Context) ([]entities.Document, error) {
	iter := s.client.Collection(collection).Documents(ctx)
	defer iter.Stop()

	var out []entities.Document
	for {
		snap, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("documents: list: %w", err)
		}
		var doc documentDoc
		if err := snap.DataTo(&doc); err != nil {
			return nil, fmt.Errorf("documents: decode: %w", err)
		}
		out = append(out, docToEntity(doc))
	}
	return out, nil
}

func (s *Store) FindByID(ctx context.Context, id string) (*entities.Document, error) {
	snap, err := s.client.Collection(collection).Doc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("documents: get by id: %w", err)
	}
	var doc documentDoc
	if err := snap.DataTo(&doc); err != nil {
		return nil, fmt.Errorf("documents: decode: %w", err)
	}
	out := docToEntity(doc)
	return &out, nil
}

func (s *Store) Create(ctx context.Context, d entities.Document) error {
	if _, err := s.client.Collection(collection).Doc(d.ID).Create(ctx, entityToDoc(d)); err != nil {
		return fmt.Errorf("documents: create: %w", err)
	}
	return nil
}

func (s *Store) Update(ctx context.Context, d entities.Document) error {
	if _, err := s.client.Collection(collection).Doc(d.ID).Set(ctx, entityToDoc(d)); err != nil {
		return fmt.Errorf("documents: update: %w", err)
	}
	return nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	if _, err := s.client.Collection(collection).Doc(id).Delete(ctx); err != nil {
		return fmt.Errorf("documents: delete: %w", err)
	}
	return nil
}

// SetAnalysis patches only the `analysis` field via Firestore's
// path-targeted Update — keeps a concurrent metadata edit safe from
// the full-doc rewrite that the multi-second Gemini call would
// otherwise produce. Passing nil writes a Firestore null, which the
// decoder turns back into a nil pointer on read.
//
// Returns a wrapped `domainerrors.ErrNotFound` when the document was
// deleted between the upstream FindByID and this Update — Firestore's
// path-targeted Update fails with codes.NotFound rather than creating
// the doc, so the caller's "concurrent delete during analyze" race
// surfaces as a clean 404 instead of a generic 500.
func (s *Store) SetAnalysis(ctx context.Context, id string, analysis *entities.DocumentAnalysis) error {
	updates := []fs.Update{{Path: "analysis", Value: analysisFromEntity(analysis)}}
	if _, err := s.client.Collection(collection).Doc(id).Update(ctx, updates); err != nil {
		if status.Code(err) == codes.NotFound {
			return fmt.Errorf("%w: document %q", domainerrorsPkg.ErrNotFound, id)
		}
		return fmt.Errorf("documents: set analysis: %w", err)
	}
	return nil
}

// CountByCategory uses an equality filter on `category_id`. Firestore
// auto-indexes single-field equality queries — no composite index needed.
func (s *Store) CountByCategory(ctx context.Context, categoryID string) (int, error) {
	iter := s.client.Collection(collection).
		Where("category_id", "==", categoryID).
		Documents(ctx)
	defer iter.Stop()

	count := 0
	for {
		_, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("documents: count by category: %w", err)
		}
		count++
	}
	return count, nil
}

// CountByLinkedExpense returns the number of documents pinned to the given
// expense. Powers the per-expense cap (max 10) on the unified attach flow.
func (s *Store) CountByLinkedExpense(ctx context.Context, expenseID string) (int, error) {
	iter := s.client.Collection(collection).
		Where("linked_expense_id", "==", expenseID).
		Documents(ctx)
	defer iter.Stop()

	count := 0
	for {
		_, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("documents: count by linked expense: %w", err)
		}
		count++
	}
	return count, nil
}

// CountByLinkedContract counts every document attached to a given
// contract.
func (s *Store) CountByLinkedContract(ctx context.Context, contractID string) (int, error) {
	iter := s.client.Collection(collection).
		Where("linked_contract_id", "==", contractID).
		Documents(ctx)
	defer iter.Stop()

	count := 0
	for {
		_, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("documents: count by linked contract: %w", err)
		}
		count++
	}
	return count, nil
}

// ListByLinkedExpense returns every document linked to the given expense,
// ordered client-side by uploaded_at asc. Equality on a single field uses
// Firestore's automatic single-field index — no composite needed.
func (s *Store) ListByLinkedExpense(ctx context.Context, expenseID string) ([]entities.Document, error) {
	iter := s.client.Collection(collection).
		Where("linked_expense_id", "==", expenseID).
		Documents(ctx)
	defer iter.Stop()

	var out []entities.Document
	for {
		snap, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("documents: list by linked expense: %w", err)
		}
		var doc documentDoc
		if err := snap.DataTo(&doc); err != nil {
			return nil, fmt.Errorf("documents: decode: %w", err)
		}
		out = append(out, docToEntity(doc))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UploadedAt.Before(out[j].UploadedAt)
	})
	return out, nil
}

func docToEntity(d documentDoc) entities.Document {
	return entities.Document{
		ID:               d.ID,
		CoproID:          d.CoproID,
		CategoryID:       d.CategoryID,
		Group:            d.Group,
		Title:            d.Title,
		Description:      d.Description,
		ObjectName:       d.ObjectName,
		ContentType:      d.ContentType,
		SizeBytes:        d.SizeBytes,
		OriginalFilename: d.OriginalFilename,
		UploadedAt:       d.UploadedAt,
		UploadedBy:       d.UploadedBy,
		LinkedExpenseID:  d.LinkedExpenseID,
		LinkedContractID: d.LinkedContractID,
		Analysis:         analysisToEntity(d.Analysis),
	}
}

func entityToDoc(d entities.Document) documentDoc {
	return documentDoc{
		ID:               d.ID,
		CoproID:          d.CoproID,
		CategoryID:       d.CategoryID,
		Group:            d.Group,
		Title:            d.Title,
		Description:      d.Description,
		ObjectName:       d.ObjectName,
		ContentType:      d.ContentType,
		SizeBytes:        d.SizeBytes,
		OriginalFilename: d.OriginalFilename,
		UploadedAt:       d.UploadedAt,
		UploadedBy:       d.UploadedBy,
		LinkedExpenseID:  d.LinkedExpenseID,
		LinkedContractID: d.LinkedContractID,
		Analysis:         analysisFromEntity(d.Analysis),
	}
}

func analysisToEntity(a *documentAnalysis) *entities.DocumentAnalysis {
	if a == nil {
		return nil
	}
	out := &entities.DocumentAnalysis{
		Kind:       entities.DocumentAnalysisKind(a.Kind),
		Confidence: a.Confidence,
		AnalyzedAt: a.AnalyzedAt,
		Model:      a.Model,
		Reason:     a.Reason,
	}
	if a.Expense != nil {
		out.Expense = &entities.ExpenseExtraction{
			AmountEUR:    a.Expense.AmountEUR,
			Date:         a.Expense.Date,
			Vendor:       a.Expense.Vendor,
			CategoryHint: a.Expense.CategoryHint,
			Description:  a.Expense.Description,
		}
	}
	if a.Contract != nil {
		out.Contract = &entities.ContractExtraction{
			Provider:         a.Contract.Provider,
			ContractType:     a.Contract.ContractType,
			StartDate:        a.Contract.StartDate,
			EndDate:          a.Contract.EndDate,
			MonthlyAmountEUR: a.Contract.MonthlyAmountEUR,
			ContractNumber:   a.Contract.ContractNumber,
		}
	}
	return out
}

func analysisFromEntity(a *entities.DocumentAnalysis) *documentAnalysis {
	if a == nil {
		return nil
	}
	out := &documentAnalysis{
		Kind:       string(a.Kind),
		Confidence: a.Confidence,
		AnalyzedAt: a.AnalyzedAt,
		Model:      a.Model,
		Reason:     a.Reason,
	}
	if a.Expense != nil {
		out.Expense = &expenseExtraction{
			AmountEUR:    a.Expense.AmountEUR,
			Date:         a.Expense.Date,
			Vendor:       a.Expense.Vendor,
			CategoryHint: a.Expense.CategoryHint,
			Description:  a.Expense.Description,
		}
	}
	if a.Contract != nil {
		out.Contract = &contractExtraction{
			Provider:         a.Contract.Provider,
			ContractType:     a.Contract.ContractType,
			StartDate:        a.Contract.StartDate,
			EndDate:          a.Contract.EndDate,
			MonthlyAmountEUR: a.Contract.MonthlyAmountEUR,
			ContractNumber:   a.Contract.ContractNumber,
		}
	}
	return out
}
