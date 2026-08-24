package httpui

import (
	"net/http"
	"strconv"
	"strings"

	"manuscript-conservation-gate/internal/application"
	"manuscript-conservation-gate/internal/domain"
)

func (s *Server) Health(w http.ResponseWriter, r *http.Request) {
	if err := s.service.VerifyStore(r.Context()); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "startedAt": s.started})
}

func (s *Server) ListCases(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	page, pageErr := positiveInt(values.Get("page"), 1)
	pageSize, sizeErr := positiveInt(values.Get("pageSize"), 25)
	if pageErr != nil {
		writeError(w, &domain.ValidationError{Fields: []domain.FieldError{{Field: "page", Message: "页码必须是正整数"}}})
		return
	}
	if sizeErr != nil {
		writeError(w, &domain.ValidationError{Fields: []domain.FieldError{{Field: "pageSize", Message: "每页数量必须是正整数"}}})
		return
	}
	status := domain.Status(strings.TrimSpace(values.Get("status")))
	conservator := values.Get("responsibleConservator")
	if conservator == "" {
		conservator = values.Get("conservator")
	}
	result, err := s.service.QueryCases(r.Context(), application.CaseQuery{Keyword: values.Get("keyword"), ResponsibleConservator: conservator, Status: status, Page: page, PageSize: pageSize})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func positiveInt(raw string, fallback int) (int, error) {
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return 0, domain.ErrValidation
	}
	return value, nil
}

func (s *Server) GetCase(w http.ResponseWriter, r *http.Request) {
	c, err := s.service.GetCase(r.Context(), r.PathValue("caseID"))
	if err != nil {
		writeError(w, err)
		return
	}
	response := map[string]any{"case": c}
	if evidence, err := c.BuildEvidenceSnapshot(); err == nil {
		response["reviewEvidence"] = evidence
	}
	if evidence, err := c.GateEvidenceSnapshot(); err == nil {
		response["releaseEvidence"] = evidence
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) CreateCase(w http.ResponseWriter, r *http.Request) {
	var command application.CreateCaseCommand
	if !decodeOrFail(w, r, &command) {
		return
	}
	result, err := s.service.CreateCase(r.Context(), command)
	if err != nil {
		writeError(w, err)
		return
	}
	status := http.StatusCreated
	if result.Replayed {
		status = http.StatusOK
	}
	writeJSON(w, status, result)
}

func (s *Server) AddDamage(w http.ResponseWriter, r *http.Request) {
	var command application.AddDamageCommand
	if !decodeOrFail(w, r, &command) {
		return
	}
	result, err := s.service.AddDamage(r.Context(), r.PathValue("caseID"), command)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) CreatePlan(w http.ResponseWriter, r *http.Request) {
	var command application.CreatePlanCommand
	if !decodeOrFail(w, r, &command) {
		return
	}
	result, err := s.service.CreatePlan(r.Context(), r.PathValue("caseID"), command)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
