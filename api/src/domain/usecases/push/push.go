// Package push owns Web Push subscription lifecycle (Subscribe /
// Unsubscribe). The actual fan-out happens inside alerts.Fire — push
// here is purely the user-facing CRUD that lets a browser register or
// drop its endpoint.
package push

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	domainerrors "github.com/titouanfreville/copro-manager/api/src/domain/errors"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

// trustedPushHosts is the allowlist of valid Web Push service endpoints
// per the major browsers' canonical push backends. Anything else and we
// refuse to register the subscription — otherwise our outbound
// fan-out becomes a generic SSRF probe (RFC8030: server signs and POSTs
// to whatever URL the client supplied).
var trustedPushHostSuffixes = []string{
	".googleapis.com",            // Chrome / Edge / Android
	".push.services.mozilla.com", // Firefox
	".push.apple.com",            // Safari (iOS 16.4+)
	".windows.com",               // Edge legacy
	".notify.windows.com",        // Edge legacy
}

const (
	endpointMaxLen  = 2048
	keyMaxLen       = 512
	userAgentMaxLen = 256
)

// SubscribeInput is the payload the browser provides post `pushManager.subscribe()`.
type SubscribeInput struct {
	ActorUserID string
	Endpoint    string
	P256dh      string
	Auth        string
	UserAgent   string
}

// Usecases is the push domain contract.
type Usecases interface {
	Subscribe(ctx context.Context, in SubscribeInput) error
	Unsubscribe(ctx context.Context, endpoint, actorUserID string) error
}

type usecases struct {
	logger *zap.Logger
	store  interfaces.PushSubscriptionsStore
	foyers interfaces.FoyersStore
	now    func() time.Time
}

func New(
	logger *zap.Logger,
	store interfaces.PushSubscriptionsStore,
	foyers interfaces.FoyersStore,
) Usecases {
	return &usecases{
		logger: logger.Named("usecases.push"),
		store:  store,
		foyers: foyers,
		now:    time.Now,
	}
}

func (uc *usecases) Subscribe(ctx context.Context, in SubscribeInput) error {
	endpoint := strings.TrimSpace(in.Endpoint)
	if endpoint == "" {
		return entities.ValidationError{Key: "endpoint", Message: "required"}
	}
	if len(endpoint) > endpointMaxLen {
		return entities.ValidationError{Key: "endpoint", Message: "endpoint too long"}
	}
	if err := validatePushEndpoint(endpoint); err != nil {
		uc.logger.Warn("rejected push endpoint", zap.Error(err))
		return err
	}
	if strings.TrimSpace(in.P256dh) == "" || strings.TrimSpace(in.Auth) == "" {
		return entities.ValidationError{Key: "keys", Message: "p256dh and auth required"}
	}
	if len(in.P256dh) > keyMaxLen || len(in.Auth) > keyMaxLen {
		return entities.ValidationError{Key: "keys", Message: "keys too long"}
	}
	if _, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(in.P256dh, "=")); err != nil {
		return entities.ValidationError{Key: "p256dh", Message: "must be base64url"}
	}
	if _, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(in.Auth, "=")); err != nil {
		return entities.ValidationError{Key: "auth", Message: "must be base64url"}
	}
	userAgent := in.UserAgent
	if len(userAgent) > userAgentMaxLen {
		userAgent = userAgent[:userAgentMaxLen]
	}

	foyer, err := uc.actorFoyer(ctx, in.ActorUserID)
	if err != nil {
		return err
	}

	// Cross-foyer clobber guard: the same browser endpoint can be reused by
	// a different user (shared device, sequential logins). Refuse to flip
	// the FoyerID on an existing subscription — the prior owner must
	// Unsubscribe first.
	existing, err := uc.store.FindByEndpoint(ctx, endpoint)
	if err != nil {
		uc.logger.Error("subscribe lookup failed", zap.Error(err))
		return fmt.Errorf("lookup existing subscription: %w", err)
	}
	if existing != nil && existing.FoyerID != foyer.ID {
		uc.logger.Warn("refusing cross-foyer endpoint takeover",
			zap.String("existing_foyer_id", existing.FoyerID),
			zap.String("new_foyer_id", foyer.ID))
		return entities.AuthorizationError{Code: "endpoint_owned_by_other_foyer"}
	}

	sub := entities.PushSubscription{
		FoyerID:   foyer.ID,
		Endpoint:  endpoint,
		P256dh:    in.P256dh,
		Auth:      in.Auth,
		UserAgent: userAgent,
		CreatedAt: uc.now(),
	}
	if err := uc.store.Upsert(ctx, sub); err != nil {
		uc.logger.Error("subscribe failed", zap.Error(err))
		return fmt.Errorf("upsert subscription: %w", err)
	}
	uc.logger.Info("Success", zap.String("foyer_id", foyer.ID))
	return nil
}

func (uc *usecases) Unsubscribe(ctx context.Context, endpoint, actorUserID string) error {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return entities.ValidationError{Key: "endpoint", Message: "required"}
	}
	foyer, err := uc.actorFoyer(ctx, actorUserID)
	if err != nil {
		return err
	}

	// Ownership check: only the foyer that registered the endpoint can
	// drop it. Without this any authed user could grief another foyer's
	// devices by submitting a guessed endpoint URL (URLs are high-entropy
	// so practically unguessable, but the missing check is still a hole).
	existing, err := uc.store.FindByEndpoint(ctx, endpoint)
	if err != nil {
		uc.logger.Error("unsubscribe lookup failed", zap.Error(err))
		return fmt.Errorf("lookup subscription: %w", err)
	}
	if existing == nil {
		// Idempotent no-op (subscription already gone or never existed).
		return nil
	}
	if existing.FoyerID != foyer.ID {
		uc.logger.Warn("refusing to unsubscribe other foyer's endpoint",
			zap.String("owner_foyer_id", existing.FoyerID),
			zap.String("actor_foyer_id", foyer.ID))
		return entities.AuthorizationError{Code: "not_subscription_owner"}
	}

	if err := uc.store.DeleteByEndpoint(ctx, endpoint); err != nil {
		uc.logger.Error("unsubscribe failed", zap.Error(err))
		return fmt.Errorf("delete subscription: %w", err)
	}
	return nil
}

// validatePushEndpoint enforces that the endpoint URL is HTTPS AND points
// at a known Web Push backend. Web Push endpoints are server-signed and
// POSTed by alerts.fanOutPush — without this guard we'd act as a generic
// outbound HTTP probe (SSRF).
func validatePushEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return entities.ValidationError{Key: "endpoint", Message: "must be a valid URL"}
	}
	if u.Scheme != "https" {
		return entities.ValidationError{Key: "endpoint", Message: "must be https"}
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return entities.ValidationError{Key: "endpoint", Message: "missing host"}
	}
	for _, suffix := range trustedPushHostSuffixes {
		if strings.HasSuffix(host, suffix) {
			return nil
		}
	}
	return entities.ValidationError{Key: "endpoint", Message: "host is not a known Web Push service"}
}

func (uc *usecases) actorFoyer(ctx context.Context, actorUserID string) (*entities.Foyer, error) {
	if actorUserID == "" {
		return nil, entities.AuthorizationError{Code: "actor_required"}
	}
	rdc, err := uc.foyers.FindByFloor(ctx, entities.FoyerFloorRDC)
	if err != nil {
		return nil, err
	}
	premier, err := uc.foyers.FindByFloor(ctx, entities.FoyerFloor1er)
	if err != nil {
		return nil, err
	}
	if rdc == nil || premier == nil {
		return nil, fmt.Errorf("%w: both foyers must exist", domainerrors.ErrNotFound)
	}
	for _, mid := range rdc.MemberIDs {
		if mid == actorUserID {
			return rdc, nil
		}
	}
	for _, mid := range premier.MemberIDs {
		if mid == actorUserID {
			return premier, nil
		}
	}
	return nil, entities.AuthorizationError{Code: "not_foyer_member"}
}
