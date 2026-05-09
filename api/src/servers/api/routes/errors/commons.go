package errors

import (
	"errors"
	"net/http"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	domainerrors "github.com/titouanfreville/copro-manager/api/src/domain/errors"
)

// ManageErrors maps domain errors to HTTP status codes and error responses.
func ManageErrors(err error) (int, ServErrors) {
	switch {
	case errors.Is(err, entities.AuthorizationError{}):
		// Domain-level rejection of an authenticated caller — 403 Forbidden
		// is the right semantic. The Authorize middleware uses 401 for
		// missing/invalid credentials before reaching the domain.
		return http.StatusForbidden, NewServErrors("FORBIDDEN", "forbidden")
	case errors.Is(err, entities.ValidationError{}):
		return http.StatusBadRequest, NewServErrors("VALIDATION_ERROR", err.Error())
	case errors.Is(err, domainerrors.ErrNotFound):
		return http.StatusNotFound, NewServErrors("NOT_FOUND", "resource not found")
	case errors.Is(err, domainerrors.ErrAlreadyExists):
		return http.StatusConflict, NewServErrors("ALREADY_EXISTS", err.Error())
	case errors.Is(err, domainerrors.ErrNotImplemented):
		return http.StatusNotImplemented, NewServErrors("NOT_IMPLEMENTED", "not implemented")
	case errors.Is(err, domainerrors.ErrFeatureDisabled):
		return http.StatusServiceUnavailable, NewServErrors("FEATURE_DISABLED", "feature désactivée")
	case errors.Is(err, domainerrors.ErrFeatureCapped):
		return http.StatusTooManyRequests, NewServErrors("FEATURE_CAPPED", "quota mensuel atteint, réessaie le mois prochain")
	default:
		return http.StatusInternalServerError, NewServErrors("INTERNAL_ERROR", "internal server error")
	}
}
