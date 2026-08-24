package httpui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"manuscript-conservation-gate/internal/domain"
)

const maxRequestBody = 1 << 20

type errorResponse struct {
	Error  string              `json:"error"`
	Code   string              `json:"code"`
	Fields []domain.FieldError `json:"fields,omitempty"`
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("请求体不能为空")
		}
		return fmt.Errorf("请求 JSON 无效: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return fmt.Errorf("请求体只能包含一个 JSON 对象")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	status, code := http.StatusInternalServerError, "INTERNAL_ERROR"
	switch {
	case errors.Is(err, domain.ErrValidation):
		status, code = http.StatusUnprocessableEntity, "VALIDATION_ERROR"
	case errors.Is(err, domain.ErrNotFound):
		status, code = http.StatusNotFound, "NOT_FOUND"
	case errors.Is(err, domain.ErrForbidden):
		status, code = http.StatusForbidden, "FORBIDDEN"
	case errors.Is(err, domain.ErrVersionConflict):
		status, code = http.StatusConflict, "VERSION_CONFLICT"
	case errors.Is(err, domain.ErrIdempotencyConflict):
		status, code = http.StatusConflict, "IDEMPOTENCY_CONFLICT"
	case errors.Is(err, domain.ErrDuplicateAccession):
		status, code = http.StatusConflict, "DUPLICATE_ACCESSION"
	case errors.Is(err, domain.ErrInvalidTransition):
		status, code = http.StatusConflict, "INVALID_TRANSITION"
	case errors.Is(err, domain.ErrIntegrity):
		status, code = http.StatusInternalServerError, "INTEGRITY_ERROR"
	}
	response := errorResponse{Error: err.Error(), Code: code}
	var validation *domain.ValidationError
	if errors.As(err, &validation) {
		response.Fields = validation.Fields
	}
	writeJSON(w, status, response)
}

func decodeOrFail(w http.ResponseWriter, r *http.Request, value any) bool {
	if err := decodeJSON(w, r, value); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "BAD_JSON"})
		return false
	}
	return true
}
