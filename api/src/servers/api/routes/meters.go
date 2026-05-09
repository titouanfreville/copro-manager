package routes

import (
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/titouanfreville/copro-manager/api/src/core/rest"
	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	"github.com/titouanfreville/copro-manager/api/src/domain/usecases/meters"
	routeerrors "github.com/titouanfreville/copro-manager/api/src/servers/api/routes/errors"
	"github.com/titouanfreville/copro-manager/api/src/servers/api/shared"
)

type saveMeterRequest struct {
	Period    string  `json:"period"`
	GlobalM3  float64 `json:"global_m3"`
	CommonM3  float64 `json:"common_m3"`
	RDCM3     float64 `json:"rdc_m3"`
	PremierM3 float64 `json:"premier_m3"`
}

func (r saveMeterRequest) toInput(actorUID, periodOverride string) meters.SaveInput {
	period := r.Period
	if periodOverride != "" {
		period = periodOverride
	}
	return meters.SaveInput{
		ActorUserID: actorUID,
		Period:      period,
		GlobalM3:    r.GlobalM3,
		CommonM3:    r.CommonM3,
		RDCM3:       r.RDCM3,
		PremierM3:   r.PremierM3,
	}
}

type meterPhotoUploadURLRequest struct {
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
}

type meterPhotoUploadURLResponse struct {
	ObjectName  string    `json:"object_name"`
	UploadURL   string    `json:"upload_url"`
	ContentType string    `json:"content_type"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type meterPhotoRecordRequest struct {
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
}

type meterPhotoDownloadURLResponse struct {
	DownloadURL string    `json:"download_url"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// ListMeters handles GET /meters.
func (e *Endpoints) ListMeters(w http.ResponseWriter, r *http.Request) {
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	out, err := e.usecases.Meters.List(r.Context(), actorUID)
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	if out == nil {
		out = []entities.MeterReading{}
	}
	rest.Render().JSON(http.StatusOK, w, r, out)
}

// GetMeter handles GET /meters/{period}.
func (e *Endpoints) GetMeter(w http.ResponseWriter, r *http.Request) {
	period := chi.URLParam(r, "period")
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	m, err := e.usecases.Meters.FindByPeriod(r.Context(), period, actorUID)
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	rest.Render().JSON(http.StatusOK, w, r, m)
}

// CreateMeter handles POST /meters.
func (e *Endpoints) CreateMeter(w http.ResponseWriter, r *http.Request) {
	var req saveMeterRequest
	if err := rest.Bind().JSONData(r, &req); err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_BODY", "invalid JSON body"))
		return
	}
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	m, err := e.usecases.Meters.Create(r.Context(), req.toInput(actorUID, ""))
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	rest.Render().JSON(http.StatusCreated, w, r, m)
}

// UpdateMeter handles PATCH /meters/{period}.
func (e *Endpoints) UpdateMeter(w http.ResponseWriter, r *http.Request) {
	period := chi.URLParam(r, "period")
	var req saveMeterRequest
	if err := rest.Bind().JSONData(r, &req); err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_BODY", "invalid JSON body"))
		return
	}
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	m, err := e.usecases.Meters.Update(r.Context(), req.toInput(actorUID, period))
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	rest.Render().JSON(http.StatusOK, w, r, m)
}

// DeleteMeter handles DELETE /meters/{period}.
func (e *Endpoints) DeleteMeter(w http.ResponseWriter, r *http.Request) {
	period := chi.URLParam(r, "period")
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	if err := e.usecases.Meters.Delete(r.Context(), period, actorUID); err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	rest.Render().NoContent(http.StatusNoContent, w)
}

// RequestMeterPhotoUploadURL handles POST /meters/{period}/photos/{kind}/upload-url.
func (e *Endpoints) RequestMeterPhotoUploadURL(w http.ResponseWriter, r *http.Request) {
	period := chi.URLParam(r, "period")
	kind := entities.MeterPhotoKind(chi.URLParam(r, "kind"))
	var req meterPhotoUploadURLRequest
	if err := rest.Bind().JSONData(r, &req); err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_BODY", "invalid JSON body"))
		return
	}
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	res, err := e.usecases.Meters.RequestPhotoUploadURL(r.Context(), meters.RequestPhotoUploadInput{
		ActorUserID: actorUID,
		Period:      period,
		Kind:        kind,
		ContentType: req.ContentType,
		SizeBytes:   req.SizeBytes,
	})
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	rest.Render().JSON(http.StatusOK, w, r, meterPhotoUploadURLResponse{
		ObjectName:  res.ObjectName,
		UploadURL:   res.UploadURL,
		ContentType: res.ContentType,
		ExpiresAt:   res.ExpiresAt,
	})
}

// RecordMeterPhoto handles POST /meters/{period}/photos/{kind}.
func (e *Endpoints) RecordMeterPhoto(w http.ResponseWriter, r *http.Request) {
	period := chi.URLParam(r, "period")
	kind := entities.MeterPhotoKind(chi.URLParam(r, "kind"))
	var req meterPhotoRecordRequest
	if err := rest.Bind().JSONData(r, &req); err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_BODY", "invalid JSON body"))
		return
	}
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	m, err := e.usecases.Meters.RecordPhoto(r.Context(), meters.RecordPhotoInput{
		ActorUserID: actorUID,
		Period:      period,
		Kind:        kind,
		ContentType: req.ContentType,
		SizeBytes:   req.SizeBytes,
	})
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	rest.Render().JSON(http.StatusOK, w, r, m)
}

// GetMeterPhotoDownloadURL handles GET /meters/{period}/photos/{kind}/download-url.
func (e *Endpoints) GetMeterPhotoDownloadURL(w http.ResponseWriter, r *http.Request) {
	period := chi.URLParam(r, "period")
	kind := entities.MeterPhotoKind(chi.URLParam(r, "kind"))
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	url, expiresAt, err := e.usecases.Meters.GetPhotoDownloadURL(r.Context(), period, kind, actorUID)
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	rest.Render().JSON(http.StatusOK, w, r, meterPhotoDownloadURLResponse{
		DownloadURL: url,
		ExpiresAt:   expiresAt,
	})
}

type meterPhotoOCRResponse struct {
	Values     []float64 `json:"values"`
	Confidence []float64 `json:"confidence"`
}

// SuggestRawMeterPhotoValues handles POST /meters/ocr/{kind} (multipart
// `file`). Stateless OCR for the capture flow — runs Vision on the
// inline bytes and returns the detected numbers without touching
// Firestore or GCS. Cap is 10 MB per image (matches the storage
// pipeline).
func (e *Endpoints) SuggestRawMeterPhotoValues(w http.ResponseWriter, r *http.Request) {
	kind := entities.MeterPhotoKind(chi.URLParam(r, "kind"))
	// MaxBytesReader caps the request body BEFORE multipart parsing so an
	// attacker can't exhaust Cloud Run /tmp by streaming a huge payload.
	// ParseMultipartForm's argument is only the in-memory ceiling.
	r.Body = http.MaxBytesReader(w, r.Body, entities.MeterReadingMaxPhotoBytes+4096)
	if err := r.ParseMultipartForm(entities.MeterReadingMaxPhotoBytes + 1024); err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("INVALID_BODY", "invalid multipart body"))
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("MISSING_FILE", "expected multipart field 'file'"))
		return
	}
	defer func() { _ = file.Close() }()
	// Cap the read at the photo limit + a small slack so a malicious
	// client can't tarpit Cloud Run with an unbounded stream.
	limited := io.LimitReader(file, entities.MeterReadingMaxPhotoBytes+1)
	bytes, err := io.ReadAll(limited)
	if err != nil {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("READ_FAILED", "could not read uploaded file"))
		return
	}
	// Sniff the magic bytes — http.DetectContentType is good enough for
	// JPEG/PNG. Reject anything not on the meter-photo allow-list BEFORE
	// the Vision call; otherwise we'd burn a paid OCR request on garbage
	// (or whatever the client decided to label image/jpeg).
	sniffed := http.DetectContentType(bytes)
	parsed, _, _ := mime.ParseMediaType(sniffed)
	if parsed == "" {
		parsed = sniffed
	}
	if !entities.IsAllowedMeterPhotoMime(parsed) {
		rest.Render().JSON(http.StatusBadRequest, w, r, routeerrors.NewServErrors("UNSUPPORTED_TYPE", "format non supporté (JPEG/PNG attendu)"))
		return
	}
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	res, err := e.usecases.Meters.SuggestRawPhotoValues(r.Context(), kind, bytes, actorUID)
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	out := meterPhotoOCRResponse{Values: res.Values, Confidence: res.Confidence}
	if out.Values == nil {
		out.Values = []float64{}
	}
	if out.Confidence == nil {
		out.Confidence = []float64{}
	}
	rest.Render().JSON(http.StatusOK, w, r, out)
}

// SuggestMeterPhotoValues handles POST /meters/{period}/photos/{kind}/ocr.
// Returns up to 1 (global) or 3 (detail) detected numeric readings the
// UI uses to pre-fill the form. Empty arrays when OCR is unavailable
// or no number-like text was found — the user types manually.
func (e *Endpoints) SuggestMeterPhotoValues(w http.ResponseWriter, r *http.Request) {
	period := chi.URLParam(r, "period")
	kind := entities.MeterPhotoKind(chi.URLParam(r, "kind"))
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	res, err := e.usecases.Meters.SuggestPhotoValues(r.Context(), period, kind, actorUID)
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	out := meterPhotoOCRResponse{Values: res.Values, Confidence: res.Confidence}
	if out.Values == nil {
		out.Values = []float64{}
	}
	if out.Confidence == nil {
		out.Confidence = []float64{}
	}
	rest.Render().JSON(http.StatusOK, w, r, out)
}

// DeleteMeterPhoto handles DELETE /meters/{period}/photos/{kind}.
func (e *Endpoints) DeleteMeterPhoto(w http.ResponseWriter, r *http.Request) {
	period := chi.URLParam(r, "period")
	kind := entities.MeterPhotoKind(chi.URLParam(r, "kind"))
	actorUID, _ := r.Context().Value(shared.UserID).(string)
	m, err := e.usecases.Meters.DeletePhoto(r.Context(), period, kind, actorUID)
	if err != nil {
		status, body := routeerrors.ManageErrors(err)
		rest.Render().JSON(status, w, r, body)
		return
	}
	rest.Render().JSON(http.StatusOK, w, r, m)
}
