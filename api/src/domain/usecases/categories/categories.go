// Package categories owns category metadata: read path used by the
// expense-creation flow plus full CRUD for foyer-managed custom
// categories. Predefined categories are seeded at boot and read-only
// except for their default distribution mode.
package categories

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	domainerrors "github.com/titouanfreville/copro-manager/api/src/domain/errors"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

const (
	nameMinLen = 2
	nameMaxLen = 40
	// iconMaxLen caps the icon string at 8 bytes — wide enough for a
	// single emoji plus a variation selector (e.g. "💧" or "🏛️" with
	// VS-16) but tight enough to refuse free-text labels.
	iconMaxLen = 8
)

// CreateCategoryInput captures user-typed fields. ID is server-generated.
type CreateCategoryInput struct {
	ActorUserID             string
	Name                    string
	DefaultDistributionMode entities.DistributionMode
	Icon                    string
	Color                   string
}

// UpdateCategoryInput is for editing an existing category. Predefined
// categories accept only DefaultDistributionMode + cosmetic fields
// (Icon, Color); custom categories accept Name on top. The
// predefined-guard lives in the usecase, not the type.
type UpdateCategoryInput struct {
	ActorUserID             string
	Name                    string
	DefaultDistributionMode entities.DistributionMode
	Icon                    string
	Color                   string
}

// Usecases is the categories domain contract.
type Usecases interface {
	FindByID(ctx context.Context, id string) (*entities.Category, error)
	Create(ctx context.Context, in CreateCategoryInput) (*entities.Category, error)
	Update(ctx context.Context, id string, in UpdateCategoryInput) (*entities.Category, error)
	Delete(ctx context.Context, id, actorUserID string) error
}

type usecases struct {
	logger    *zap.Logger
	store     interfaces.CategoriesStore
	expenses  interfaces.ExpensesStore
	templates interfaces.TemplatesStore
	documents interfaces.DocumentsStore
	contracts interfaces.ContractsStore
	foyers    interfaces.FoyersStore
}

// New builds a categories usecase. The reference-count check on Delete
// queries expenses + templates + documents + contracts — those stores
// are injected so the usecase doesn't reach across packages directly.
func New(
	logger *zap.Logger,
	store interfaces.CategoriesStore,
	expenses interfaces.ExpensesStore,
	templates interfaces.TemplatesStore,
	documents interfaces.DocumentsStore,
	contracts interfaces.ContractsStore,
	foyers interfaces.FoyersStore,
) Usecases {
	return &usecases{
		logger:    logger.Named("usecases.categories"),
		store:     store,
		expenses:  expenses,
		templates: templates,
		documents: documents,
		contracts: contracts,
		foyers:    foyers,
	}
}

func (uc *usecases) FindByID(ctx context.Context, id string) (*entities.Category, error) {
	return uc.store.FindByID(ctx, id)
}

func (uc *usecases) Create(ctx context.Context, in CreateCategoryInput) (*entities.Category, error) {
	log := uc.logger.With(zap.String("method", "Create"))

	if err := uc.authorize(ctx, in.ActorUserID); err != nil {
		log.Warn("actor unauthorized", zap.Error(err))
		return nil, err
	}
	name, err := normalizeName(in.Name)
	if err != nil {
		return nil, err
	}
	if in.DefaultDistributionMode != "" && !entities.IsKnownDistributionMode(in.DefaultDistributionMode) {
		return nil, entities.ValidationError{Key: "default_distribution_mode", Message: "unknown mode"}
	}
	icon, err := normalizeIcon(in.Icon)
	if err != nil {
		return nil, err
	}
	color, err := normalizeColor(in.Color)
	if err != nil {
		return nil, err
	}

	// Case-insensitive uniqueness check. Listing the full set is fine at
	// 2-foyer scale (< 30 categories ever). No new index required.
	existing, err := uc.store.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	lower := strings.ToLower(name)
	for _, c := range existing {
		if strings.ToLower(c.Name) == lower {
			return nil, entities.ValidationError{Key: "name", Message: "déjà utilisée"}
		}
	}

	c := entities.Category{
		ID:                      uuid.NewString(),
		Name:                    name,
		Predefined:              false,
		Hidden:                  false,
		DefaultDistributionMode: in.DefaultDistributionMode,
		Icon:                    icon,
		Color:                   color,
	}
	if err := uc.store.Create(ctx, c); err != nil {
		log.Error("store create failed", zap.Error(err))
		return nil, fmt.Errorf("create category: %w", err)
	}
	log.Info("Success", zap.String("category_id", c.ID))
	return &c, nil
}

func (uc *usecases) Update(ctx context.Context, id string, in UpdateCategoryInput) (*entities.Category, error) {
	log := uc.logger.With(zap.String("method", "Update"), zap.String("category_id", id))

	if err := uc.authorize(ctx, in.ActorUserID); err != nil {
		return nil, err
	}
	existing, err := uc.store.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("find category: %w", err)
	}
	if existing == nil {
		return nil, fmt.Errorf("%w: category %q", domainerrors.ErrNotFound, id)
	}

	if in.DefaultDistributionMode != "" && !entities.IsKnownDistributionMode(in.DefaultDistributionMode) {
		return nil, entities.ValidationError{Key: "default_distribution_mode", Message: "unknown mode"}
	}
	icon, err := normalizeIcon(in.Icon)
	if err != nil {
		return nil, err
	}
	color, err := normalizeColor(in.Color)
	if err != nil {
		return nil, err
	}

	if existing.Predefined {
		// Predefined categories are read-only except for the default
		// distribution mode (PRD FR12) and the cosmetic icon/color
		// fields, which the user can tailor without changing identity.
		existing.DefaultDistributionMode = in.DefaultDistributionMode
		existing.Icon = icon
		existing.Color = color
	} else {
		name, err := normalizeName(in.Name)
		if err != nil {
			return nil, err
		}
		// Case-insensitive uniqueness — exclude the current row from the
		// collision check (renaming to its own current name should pass).
		all, err := uc.store.List(ctx)
		if err != nil {
			return nil, fmt.Errorf("list categories: %w", err)
		}
		lower := strings.ToLower(name)
		for _, c := range all {
			if c.ID == id {
				continue
			}
			if strings.ToLower(c.Name) == lower {
				return nil, entities.ValidationError{Key: "name", Message: "déjà utilisée"}
			}
		}
		existing.Name = name
		existing.DefaultDistributionMode = in.DefaultDistributionMode
		existing.Icon = icon
		existing.Color = color
	}

	if err := uc.store.Update(ctx, *existing); err != nil {
		log.Error("update failed", zap.Error(err))
		return nil, fmt.Errorf("update category: %w", err)
	}
	log.Info("Success")
	return existing, nil
}

func (uc *usecases) Delete(ctx context.Context, id, actorUserID string) error {
	log := uc.logger.With(zap.String("method", "Delete"), zap.String("category_id", id))

	if err := uc.authorize(ctx, actorUserID); err != nil {
		return err
	}
	existing, err := uc.store.FindByID(ctx, id)
	if err != nil {
		return fmt.Errorf("find category: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("%w: category %q", domainerrors.ErrNotFound, id)
	}
	if existing.Predefined {
		return entities.ValidationError{Key: "predefined", Message: "predefined categories cannot be deleted"}
	}

	expCount, err := uc.expenses.CountByCategory(ctx, id)
	if err != nil {
		return fmt.Errorf("count expenses: %w", err)
	}
	tplCount, err := uc.templates.CountByCategory(ctx, id)
	if err != nil {
		return fmt.Errorf("count templates: %w", err)
	}
	docCount, err := uc.documents.CountByCategory(ctx, id)
	if err != nil {
		return fmt.Errorf("count documents: %w", err)
	}
	contractCount, err := uc.contracts.CountByCategory(ctx, id)
	if err != nil {
		return fmt.Errorf("count contracts: %w", err)
	}
	if total := expCount + tplCount + docCount + contractCount; total > 0 {
		parts := []string{}
		if expCount > 0 {
			parts = append(parts, fmt.Sprintf("%d dépense(s)", expCount))
		}
		if tplCount > 0 {
			parts = append(parts, fmt.Sprintf("%d modèle(s)", tplCount))
		}
		if docCount > 0 {
			parts = append(parts, fmt.Sprintf("%d document(s)", docCount))
		}
		if contractCount > 0 {
			parts = append(parts, fmt.Sprintf("%d contrat(s)", contractCount))
		}
		return entities.ValidationError{
			Key:     "category",
			Message: "utilisée par " + strings.Join(parts, ", "),
		}
	}

	if err := uc.store.Delete(ctx, id); err != nil {
		log.Error("delete failed", zap.Error(err))
		return fmt.Errorf("delete category: %w", err)
	}
	log.Info("Success")
	return nil
}

// EnsureSeeded provisions the predefined categories. Called once at app boot
// from bin/app/app.go via fx.Invoke; idempotent.
func EnsureSeeded(ctx context.Context, store interfaces.CategoriesStore) error {
	return store.EnsureSeeded(ctx, entities.PredefinedCategories)
}

// authorize replicates the foyer-member gate used elsewhere. Empty actor
// short-circuits (no admin/cron callers for categories today).
func (uc *usecases) authorize(ctx context.Context, actorUserID string) error {
	if actorUserID == "" {
		return nil
	}
	rdc, err := uc.foyers.FindByFloor(ctx, entities.FoyerFloorRDC)
	if err != nil {
		return fmt.Errorf("find rdc: %w", err)
	}
	premier, err := uc.foyers.FindByFloor(ctx, entities.FoyerFloor1er)
	if err != nil {
		return fmt.Errorf("find 1er: %w", err)
	}
	if rdc == nil || premier == nil {
		return fmt.Errorf("%w: both foyers must exist", domainerrors.ErrNotFound)
	}
	for _, mid := range rdc.MemberIDs {
		if mid == actorUserID {
			return nil
		}
	}
	for _, mid := range premier.MemberIDs {
		if mid == actorUserID {
			return nil
		}
	}
	return entities.AuthorizationError{Code: "not_foyer_member"}
}

func normalizeName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if len(name) < nameMinLen {
		return "", entities.ValidationError{Key: "name", Message: fmt.Sprintf("min %d caractères", nameMinLen)}
	}
	if len(name) > nameMaxLen {
		return "", entities.ValidationError{Key: "name", Message: fmt.Sprintf("max %d caractères", nameMaxLen)}
	}
	return name, nil
}

// normalizeIcon trims whitespace and caps the byte length. Empty is
// allowed and means "use the frontend monogram fallback". The cap stops
// abusive payloads (full GIF embedded as text, etc.) — emojis stay well
// under it.
func normalizeIcon(raw string) (string, error) {
	icon := strings.TrimSpace(raw)
	if icon == "" {
		return "", nil
	}
	if len(icon) > iconMaxLen {
		return "", entities.ValidationError{Key: "icon", Message: fmt.Sprintf("max %d octets", iconMaxLen)}
	}
	return icon, nil
}

// normalizeColor enforces strict `#RRGGBB` hex. Empty is allowed and
// means "use the frontend predefined-palette fallback". Lowercase is the
// stored canonical form so display variants merge.
func normalizeColor(raw string) (string, error) {
	color := strings.TrimSpace(raw)
	if color == "" {
		return "", nil
	}
	if len(color) != 7 || color[0] != '#' {
		return "", entities.ValidationError{Key: "color", Message: "format attendu : #RRGGBB"}
	}
	for _, r := range color[1:] {
		ok := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
		if !ok {
			return "", entities.ValidationError{Key: "color", Message: "format attendu : #RRGGBB"}
		}
	}
	return strings.ToLower(color), nil
}
