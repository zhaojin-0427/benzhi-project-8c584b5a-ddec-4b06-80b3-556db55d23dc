package application

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"manuscript-conservation-gate/internal/audit"
	"manuscript-conservation-gate/internal/domain"
)

type mutation func(*domain.ConservationCase, time.Time) (domain.Event, *domain.ReleaseCredential, error)

func (s *Service) mutate(ctx context.Context, caseID, action string, meta CommandMeta, payload any, roles []domain.Role, change mutation) (OperationResult, error) {
	if strings.TrimSpace(caseID) == "" {
		return OperationResult{}, invalidInput("caseID", "档案标识不能为空")
	}
	if err := validateMeta(meta, false); err != nil {
		return OperationResult{}, err
	}
	if err := domain.RequireRole(meta.Role, roles...); err != nil {
		return OperationResult{}, err
	}
	payloadHash, err := domain.HashJSON(payload)
	if err != nil {
		return OperationResult{}, err
	}
	scope := caseID + ":" + action
	if result, found, err := s.repository.LookupIdempotency(ctx, scope, meta.IdempotencyKey, payloadHash); found || err != nil {
		return decodeResult(result, err)
	}
	c, err := s.repository.Get(ctx, caseID)
	if err != nil {
		return OperationResult{}, err
	}
	if c.Version != meta.ExpectedVersion {
		return OperationResult{}, domain.ErrVersionConflict
	}
	now := s.now()
	event, credentialMarker, err := change(c, now)
	if err != nil {
		return OperationResult{}, err
	}
	audit.Append(c, event)
	var credential *domain.ReleaseCredential
	if credentialMarker != nil {
		issued, err := s.issuer.Issue(s.newID("cred"), c, meta.Actor, now)
		if err != nil {
			return OperationResult{}, err
		}
		c.Credential = &issued
		credential = &issued
	}
	return s.commit(ctx, scope, payloadHash, meta, c, event, credential)
}

func (s *Service) commit(ctx context.Context, scope, payloadHash string, meta CommandMeta, c *domain.ConservationCase, event domain.Event, credential *domain.ReleaseCredential) (OperationResult, error) {
	result := OperationResult{Case: c.Clone(), Credential: credential}
	response, err := json.Marshal(result)
	if err != nil {
		return OperationResult{}, err
	}
	committed, err := s.repository.Commit(ctx, domain.CommitRequest{Scope: scope, IdempotencyKey: meta.IdempotencyKey,
		PayloadHash: payloadHash, ExpectedVersion: meta.ExpectedVersion, Case: c, Event: event, Response: response})
	return decodeResult(committed, err)
}

func decodeResult(result domain.CommitResult, err error) (OperationResult, error) {
	if err != nil {
		return OperationResult{}, err
	}
	var response OperationResult
	if err := json.Unmarshal(result.Response, &response); err != nil {
		return OperationResult{}, err
	}
	response.Replayed = result.Duplicate
	return response, nil
}

func validateMeta(meta CommandMeta, create bool) error {
	if meta.ExpectedVersion < 0 || (!create && meta.ExpectedVersion == 0) {
		return invalidInput("expectedVersion", "expectedVersion 必须是当前正整数版本")
	}
	if len(strings.TrimSpace(meta.IdempotencyKey)) < 8 || len(meta.IdempotencyKey) > 128 {
		return invalidInput("idempotencyKey", "幂等键长度应为 8 至 128 个字符")
	}
	if strings.TrimSpace(meta.Actor) == "" {
		return invalidInput("actor", "操作者不能为空")
	}
	if !domain.ValidRole(meta.Role) {
		return invalidInput("role", "角色取值无效")
	}
	return nil
}

func normalizeReason(reason, fallback string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return fallback
	}
	return reason
}

func invalidInput(field, message string) error {
	return &domain.ValidationError{Fields: []domain.FieldError{{Field: field, Message: message}}}
}
