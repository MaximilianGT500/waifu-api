package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

type Problem struct {
	Type     string              `json:"type"`
	Title    string              `json:"title"`
	Status   int                 `json:"status"`
	Detail   string              `json:"detail,omitempty"`
	Instance string              `json:"instance,omitempty"`
	TraceID  string              `json:"traceId,omitempty"`
	Errors   map[string][]string `json:"errors,omitempty"`
}

const (
	typeBadRequest   = "https://tools.ietf.org/html/rfc9110#section-15.5.1"
	typeUnauthorized = "https://tools.ietf.org/html/rfc9110#section-15.5.2"
	typeForbidden    = "https://tools.ietf.org/html/rfc9110#section-15.5.4"
	typeNotFound     = "https://tools.ietf.org/html/rfc9110#section-15.5.5"
	typeConflict     = "https://tools.ietf.org/html/rfc9110#section-15.5.10"
	typeTooLarge     = "https://tools.ietf.org/html/rfc9110#section-15.5.14"
	typeInternal     = "https://tools.ietf.org/html/rfc9110#section-15.6.1"
)

func WriteProblem(w http.ResponseWriter, r *http.Request, status int, typ, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Problem{
		Type:     typ,
		Title:    title,
		Status:   status,
		Detail:   detail,
		Instance: r.URL.Path,
		TraceID:  TraceID(r),
	})
}

func WriteValidationProblem(w http.ResponseWriter, r *http.Request, errors map[string][]string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(Problem{
		Type:     typeBadRequest,
		Title:    "One or more validation errors occurred.",
		Status:   http.StatusBadRequest,
		Detail:   "See errors property for details.",
		Instance: r.URL.Path,
		TraceID:  TraceID(r),
		Errors:   errors,
	})
}

func BadRequest(w http.ResponseWriter, r *http.Request, detail string) {
	WriteProblem(w, r, http.StatusBadRequest, typeBadRequest, "Bad Request", detail)
}

func InvalidOperation(w http.ResponseWriter, r *http.Request, detail string) {
	WriteProblem(w, r, http.StatusBadRequest, typeBadRequest, "Invalid Operation", detail)
}

func Unauthorized(w http.ResponseWriter, r *http.Request, detail string) {
	if detail == "" {
		detail = "You are not authorized to access this resource. Please provide a valid X-Api-Key header or Bearer token."
	}
	WriteProblem(w, r, http.StatusUnauthorized, typeUnauthorized, "Unauthorized", detail)
}

func Forbidden(w http.ResponseWriter, r *http.Request, detail string) {
	if detail == "" {
		detail = "You do not have permission to access this resource."
	}
	WriteProblem(w, r, http.StatusForbidden, typeForbidden, "Forbidden", detail)
}

func NotFound(w http.ResponseWriter, r *http.Request, detail string) {
	WriteProblem(w, r, http.StatusNotFound, typeNotFound, "Resource Not Found", detail)
}

func Conflict(w http.ResponseWriter, r *http.Request, detail string) {
	WriteProblem(w, r, http.StatusConflict, typeConflict, "Conflict", detail)
}

func PayloadTooLarge(w http.ResponseWriter, r *http.Request, detail string) {
	WriteProblem(w, r, http.StatusRequestEntityTooLarge, typeTooLarge, "Request Entity Too Large", detail)
}

func InternalError(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error) {
	if log != nil {
		log.Error("unhandled exception while processing request", "error", err, "path", r.URL.Path, "traceId", RequestID(r))
	}
	WriteProblem(w, r, http.StatusInternalServerError, typeInternal,
		"An error occurred while processing your request.",
		"An unexpected error occurred. Please try again later.")
}

func TraceID(r *http.Request) string {
	return RequestID(r)
}

func pascalCaseChallenge(w http.ResponseWriter, title string, status int, detail, typ string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"Title":  title,
		"Status": status,
		"Detail": detail,
		"Type":   typ,
	})
}

func WriteUnauthorizedChallenge(w http.ResponseWriter, apiKey bool) {
	if apiKey {
		pascalCaseChallenge(w, "Unauthorized", http.StatusUnauthorized,
			"You are not authorized to access this resource. Please provide a valid X-Api-Key header or Bearer token.",
			"https://tools.ietf.org/html/rfc9110#section-15.5.2")
		return
	}
	w.Header().Set("WWW-Authenticate", "Bearer")
	w.WriteHeader(http.StatusUnauthorized)
}

func WriteForbiddenChallenge(w http.ResponseWriter, apiKey bool) {
	if apiKey {
		pascalCaseChallenge(w, "Forbidden", http.StatusForbidden,
			"You do not have permission to access this resource.",
			"https://tools.ietf.org/html/rfc9110#section-15.5.4")
		return
	}
	w.WriteHeader(http.StatusForbidden)
}

func EmptyNotFound(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNotFound)
}

func RouteNotFound(w http.ResponseWriter, r *http.Request) {
	WriteProblem(w, r, http.StatusNotFound,
		"https://tools.ietf.org/html/rfc9110#section-15.5.5",
		"Not Found",
		"The requested endpoint was not found.")
}

type notFoundError struct{ msg string }

func (e *notFoundError) Error() string { return e.msg }

type conflictError struct{ msg string }

func (e *conflictError) Error() string { return e.msg }

type validationError struct{ msg string }

func (e *validationError) Error() string { return e.msg }

func errNotFound(msg string) error   { return &notFoundError{msg} }
func errConflict(msg string) error   { return &conflictError{msg} }
func errValidation(msg string) error { return &validationError{msg} }

type apiError struct {
	status     int
	typ        string
	title      string
	detail     string
	validation map[string][]string
}

func (e *apiError) Error() string { return e.detail }

func errFieldValidation(field, msg string) error {
	return &apiError{status: http.StatusBadRequest, validation: map[string][]string{field: {msg}}}
}

func errInvalidOperation(msg string) error {
	return &apiError{status: http.StatusBadRequest, typ: typeBadRequest, title: "Invalid Operation", detail: msg}
}

func errBadRequest(msg string) error {
	return &apiError{status: http.StatusBadRequest, typ: typeBadRequest, title: "Bad Request", detail: msg}
}

func errPayloadTooLarge(msg string) error {
	return &apiError{status: http.StatusRequestEntityTooLarge, typ: typeTooLarge, title: "Request Entity Too Large", detail: msg}
}

func (s *Server) writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	var nf *notFoundError
	var cf *conflictError
	var ve *validationError
	var ae *apiError
	switch {
	case errors.As(err, &ae):
		if ae.validation != nil {
			WriteValidationProblem(w, r, ae.validation)
			return
		}
		WriteProblem(w, r, ae.status, ae.typ, ae.title, ae.detail)
	case errors.As(err, &nf):
		NotFound(w, r, nf.msg)
	case errors.As(err, &cf):
		Conflict(w, r, cf.msg)
	case errors.As(err, &ve):
		BadRequest(w, r, ve.msg)
	default:
		InternalError(w, r, s.log, err)
	}
}
