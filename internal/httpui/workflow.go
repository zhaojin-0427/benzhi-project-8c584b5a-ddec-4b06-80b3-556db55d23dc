package httpui

import (
	"net/http"

	"manuscript-conservation-gate/internal/application"
	"manuscript-conservation-gate/internal/domain"
)

func (s *Server) SubmitPlan(w http.ResponseWriter, r *http.Request) {
	var command application.SubmitPlanCommand
	if !decodeOrFail(w, r, &command) {
		return
	}
	result, err := s.service.SubmitPlan(r.Context(), r.PathValue("caseID"), command)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) AssessCompatibility(w http.ResponseWriter, r *http.Request) {
	var command application.AssessCommand
	if !decodeOrFail(w, r, &command) {
		return
	}
	result, err := s.service.AssessCompatibility(r.Context(), r.PathValue("caseID"), command)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) RecordSample(w http.ResponseWriter, r *http.Request) {
	var command application.RecordSampleCommand
	if !decodeOrFail(w, r, &command) {
		return
	}
	result, err := s.service.RecordSample(r.Context(), r.PathValue("caseID"), command)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) Review(w http.ResponseWriter, r *http.Request) {
	var command application.ReviewCommand
	if !decodeOrFail(w, r, &command) {
		return
	}
	if command.Decision == domain.ReviewApproved && (command.PlanRevisionID == "" || command.PlanContentHash == "" || command.EvidenceDigest == "" || len(command.ConfirmedEvidenceItemIDs) == 0) {
		writeError(w, &domain.ValidationError{Fields: []domain.FieldError{{Field: "evidence", Message: "批准必须携带当前方案摘要和完整证据确认项"}}})
		return
	}
	result, err := s.service.Review(r.Context(), r.PathValue("caseID"), command)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) Release(w http.ResponseWriter, r *http.Request) {
	var command application.ReleaseCommand
	if !decodeOrFail(w, r, &command) {
		return
	}
	if command.PlanRevisionID == "" || command.PlanContentHash == "" || command.EvidenceDigest == "" {
		writeError(w, &domain.ValidationError{Fields: []domain.FieldError{{Field: "evidence", Message: "放行必须确认获批方案标识、方案摘要和门禁证据摘要"}}})
		return
	}
	result, err := s.service.Release(r.Context(), r.PathValue("caseID"), command)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) VerifyCredential(w http.ResponseWriter, r *http.Request) {
	result, err := s.service.VerifyCredential(r.Context(), r.PathValue("caseID"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
