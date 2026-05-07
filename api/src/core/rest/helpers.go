package rest

import (
	"net/http"
	"regexp"
	"strings"
)

var multipartRegex = regexp.MustCompile("multipart/form-data.*")

// IsMultipartFormData checks if the content type is multipart form data.
func IsMultipartFormData(h http.Header) bool {
	return multipartRegex.MatchString(h.Get("Content-Type"))
}

// IsJSONData checks if the content type is JSON.
func IsJSONData(h http.Header) bool {
	return strings.Contains(h.Get("Content-Type"), "application/json")
}
