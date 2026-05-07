package rest

import (
	"net/http"
	"net/url"
)

// Renderer defines methods for writing HTTP responses.
type Renderer interface {
	// JSON renders the provided object as a JSON response with the given status code.
	JSON(status int, w http.ResponseWriter, r *http.Request, obj interface{})

	// NoContent renders an empty response with the given status code.
	NoContent(status int, w http.ResponseWriter)
}

// Binder defines methods for parsing request data.
type Binder interface {
	// JSONData parses JSON request body into the provided structs.
	JSONData(r *http.Request, dataList ...interface{}) error

	// Form binds form data to the provided struct.
	Form(formData url.Values, obj interface{}) error

	// FormData parses and binds form data from the request to the provided structs.
	FormData(r *http.Request, dataList ...interface{}) error

	// RequestData binds JSON or form data depending on content type.
	RequestData(r *http.Request, dataList ...interface{}) error

	// URLParams binds chi URL parameters to the provided map of key -> pointer pairs.
	URLParams(r *http.Request, params map[string]interface{}) error

	// URLParam binds a single chi URL parameter to the provided pointer.
	// Returns an error if the param does not exist or is empty.
	URLParam(r *http.Request, key string, to interface{}) error

	// URLArgs binds URL query arguments to the provided map of key -> pointer pairs.
	URLArgs(r *http.Request, params map[string]interface{}) error

	// URLArg binds a single URL query argument to the provided pointer.
	// Leaves the value empty if the argument does not exist.
	URLArg(r *http.Request, key string, to interface{}) error
}
