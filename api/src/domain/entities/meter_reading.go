package entities

import (
	"regexp"
	"time"
)

// MeterReadingMaxPhotoBytes caps a meter photo at 10 MB — same ceiling as
// expense attachments / standalone documents so users hit one consistent
// limit across uploads.
const MeterReadingMaxPhotoBytes int64 = 10 * 1024 * 1024

// AllowedMeterPhotoMimeTypes whitelists images only — no PDFs (a meter
// photo is a photograph, not a scan). Map values are the canonical
// extension used when minting GCS object keys. HEIC/HEIF are
// intentionally excluded: image.Decode in the OCR pipeline only
// registers JPEG and PNG, and the SvelteKit client converts HEIC to
// JPEG via prepareForUpload before reaching this code path.
var AllowedMeterPhotoMimeTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
}

// IsAllowedMeterPhotoMime reports whether the supplied content-type is an
// accepted meter photo format.
func IsAllowedMeterPhotoMime(mime string) bool {
	_, ok := AllowedMeterPhotoMimeTypes[mime]
	return ok
}

// MeterPhotoExtension returns the canonical extension for a whitelisted
// meter photo MIME type, or "" if unknown.
func MeterPhotoExtension(mime string) string {
	return AllowedMeterPhotoMimeTypes[mime]
}

// MeterPhotoKind disambiguates the two photos a reading session captures:
// one of the global building meter, one of the panel showing the three
// detail submeters.
type MeterPhotoKind string

const (
	// MeterPhotoKindGlobal is the photo of the building's main water meter
	// (what the water company bills against).
	MeterPhotoKindGlobal MeterPhotoKind = "global"
	// MeterPhotoKindDetail is the photo of the panel showing the three
	// detail submeters (common + RDC + 1er).
	MeterPhotoKindDetail MeterPhotoKind = "detail"
)

// IsKnownMeterPhotoKind reports whether the value is one of the supported
// kinds.
func IsKnownMeterPhotoKind(k MeterPhotoKind) bool {
	switch k {
	case MeterPhotoKindGlobal, MeterPhotoKindDetail:
		return true
	}
	return false
}

// meterPeriodPattern enforces the YYYY-MM shape of a reading period.
var meterPeriodPattern = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

// IsValidMeterPeriod reports whether the value is a well-formed YYYY-MM
// period string.
func IsValidMeterPeriod(p string) bool {
	return meterPeriodPattern.MatchString(p)
}

// MeterReading is one calendar month's worth of meter snapshots — the
// global building meter plus the three detail submeters used by the
// `water_3_meters` distribution mode.
//
// Period (YYYY-MM) is the upsert key: at most one reading exists per
// month, and re-capturing edits the existing row in place. Submeters
// were installed after the global meter, so the absolute reading offset
// between `global` and `common + rdc + 1er` is baked in by design — any
// drift check the UI surfaces is therefore on month-over-month deltas,
// not on absolute readings.
//
// Two photos hang off each reading: one of the global meter, one of the
// detail panel. Both live in GCS (object names recorded here, blobs
// stored under the prefix returned by MeterPhotoPrefix).
type MeterReading struct {
	ID        string  `json:"id"`
	CoproID   string  `json:"copro_id"`
	Period    string  `json:"period"`    // "YYYY-MM"
	GlobalM3  float64 `json:"global_m3"` // building's main water meter, in m³
	CommonM3  float64 `json:"common_m3"` // common-area submeter
	RDCM3     float64 `json:"rdc_m3"`
	PremierM3 float64 `json:"premier_m3"`

	GlobalPhotoObject      string `json:"global_photo_object,omitempty"`
	GlobalPhotoContentType string `json:"global_photo_content_type,omitempty"`
	GlobalPhotoSizeBytes   int64  `json:"global_photo_size_bytes,omitempty"`
	DetailPhotoObject      string `json:"detail_photo_object,omitempty"`
	DetailPhotoContentType string `json:"detail_photo_content_type,omitempty"`
	DetailPhotoSizeBytes   int64  `json:"detail_photo_size_bytes,omitempty"`

	CapturedAt    time.Time `json:"captured_at"`
	CapturedByUID string    `json:"captured_by_uid"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// MeterPhotoPrefix is the GCS prefix where a period's photos live.
// Using the period (not a UUID) as the key segment lets a re-upload
// overwrite cleanly and keeps the cascade-delete trivial.
func MeterPhotoPrefix(period string) string {
	return "meters/" + period + "/"
}

// MeterPhotoObjectName composes the canonical GCS key for a single
// photo. Server authoritative — clients never get to choose the path.
func MeterPhotoObjectName(period string, kind MeterPhotoKind, contentType string) string {
	return MeterPhotoPrefix(period) + string(kind) + MeterPhotoExtension(contentType)
}
