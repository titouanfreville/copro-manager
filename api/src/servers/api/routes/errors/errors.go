package errors

// ServErrors is the standard error response DTO.
type ServErrors struct {
	Errors []ServError `json:"errors"`
}

// ServError represents a single error in the response.
type ServError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NewServErrors creates a new ServErrors with a single error.
func NewServErrors(code string, message string) ServErrors {
	return ServErrors{
		Errors: []ServError{
			{Code: code, Message: message},
		},
	}
}
