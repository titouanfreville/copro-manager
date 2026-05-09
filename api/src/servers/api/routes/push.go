package routes

import (
	"net/http"

	"github.com/titouanfreville/copro-manager/api/src/core/rest"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/push"
	routeerrors "github.com/titouanfreville/copro-manager/api/src/servers/api/routes/errors"
	"github.com/titouanfreville/copro-manager/api/src/servers/api/shared"
)

type pushSubscribeRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
	UserAgent string `json:"user_agent,omitempty"`
}

type pushUnsubscribeRequest struct {
	Endpoint string `json:"endpoint"`
}

// PushSubscribe handles POST /push/subscribe.
func (e *Endpoints) PushSubscribe(w http.ResponseWriter, r *http.Request) {
	var req pushSubscribeRequest
	if err := rest.Bind().JSONData(r, &req); err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_BODY", "invalid JSON body"))
		return
	}
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	err := e.usecases.Push.Subscribe(r.Context(), push.SubscribeInput{
		ActorUserID: actorUID,
		Endpoint:    req.Endpoint,
		P256dh:      req.Keys.P256dh,
		Auth:        req.Keys.Auth,
		UserAgent:   req.UserAgent,
	})
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	rest.Render().NoContent(http.StatusNoContent, w)
}

// PushUnsubscribe handles POST /push/unsubscribe.
func (e *Endpoints) PushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	var req pushUnsubscribeRequest
	if err := rest.Bind().JSONData(r, &req); err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_BODY", "invalid JSON body"))
		return
	}
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	if err := e.usecases.Push.Unsubscribe(r.Context(), req.Endpoint, actorUID); err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	rest.Render().NoContent(http.StatusNoContent, w)
}
