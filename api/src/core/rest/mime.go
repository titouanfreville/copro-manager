package rest

import (
	"mime"
	"strings"
)

// AllowedUploadMimeTypes is the canonical whitelist of content types
// the server accepts on signed-URL uploads. Map values are the file
// extension we use when minting the GCS object key — server
// authoritative so a client can't choose ".html" for an image.
//
// HEIC/HEIF are intentionally excluded: the SvelteKit client converts
// iPhone HEIC uploads to JPEG via prepareForUpload before requesting
// a signed URL, so the server only ever sees normalized formats.
var AllowedUploadMimeTypes = map[string]string{
	"image/jpeg":      ".jpg",
	"image/png":       ".png",
	"application/pdf": ".pdf",
}

// IsAllowedUploadMime reports whether the supplied content-type is in
// the whitelist (after stripping charset/boundary parameters).
func IsAllowedUploadMime(contentType string) bool {
	_, ok := AllowedUploadMimeTypes[normalize(contentType)]
	return ok
}

// UploadExtension returns the canonical extension for a whitelisted
// MIME type, or "" when unknown.
func UploadExtension(contentType string) string {
	return AllowedUploadMimeTypes[normalize(contentType)]
}

// NormalizeUploadMime parses the supplied content-type, lowercases
// it, and strips parameters. Returns the normalized value plus a
// boolean indicating whether the result is in the whitelist.
func NormalizeUploadMime(raw string) (string, bool) {
	parsed := normalize(raw)
	_, ok := AllowedUploadMimeTypes[parsed]
	return parsed, ok
}

func normalize(raw string) string {
	parsed, _, err := mime.ParseMediaType(raw)
	if err != nil {
		parsed = strings.TrimSpace(raw)
	}
	return strings.ToLower(parsed)
}
