package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"manuscript-conservation-gate/internal/audit"
	"manuscript-conservation-gate/internal/domain"
)

type Service struct {
	repository Repository
	issuer     *audit.Issuer
	now        func() time.Time
	newID      func(string) string
	flightMu   sync.Mutex
	flights    map[string]*operationFlight
}

type operationFlight struct {
	done   chan struct{}
	result OperationResult
	err    error
}

func NewService(repository Repository, issuer *audit.Issuer) *Service {
	return &Service{repository: repository, issuer: issuer, now: func() time.Time { return time.Now().UTC() }, newID: randomID, flights: map[string]*operationFlight{}}
}

func (s *Service) beginFlight(identity string) (*operationFlight, bool) {
	s.flightMu.Lock()
	defer s.flightMu.Unlock()
	if existing, ok := s.flights[identity]; ok {
		return existing, false
	}
	flight := &operationFlight{done: make(chan struct{})}
	s.flights[identity] = flight
	return flight, true
}

func (s *Service) endFlight(identity string, flight *operationFlight, result OperationResult, err error) {
	flight.result = result
	flight.err = err
	close(flight.done)
	s.flightMu.Lock()
	if s.flights[identity] == flight {
		delete(s.flights, identity)
	}
	s.flightMu.Unlock()
}

// waitForFlight observes the leader's in-flight commit, returning its final
// persisted result. If the caller's context is cancelled before the leader
// finishes, the context error is returned instead; callers never see an
// un-persisted success result.
func waitForFlight(ctx context.Context, flight *operationFlight) (OperationResult, error) {
	select {
	case <-flight.done:
		return flight.result, flight.err
	case <-ctx.Done():
		return OperationResult{}, ctx.Err()
	}
}

func (s *Service) GetCase(ctx context.Context, id string) (*domain.ConservationCase, error) {
	return s.repository.Get(ctx, id)
}

func (s *Service) ListCases(ctx context.Context) ([]*domain.ConservationCase, error) {
	return s.repository.List(ctx)
}

func (s *Service) QueryCases(ctx context.Context, query CaseQuery) (CasePage, error) {
	query.Keyword = strings.TrimSpace(query.Keyword)
	query.ResponsibleConservator = strings.TrimSpace(query.ResponsibleConservator)
	if query.Page < 1 {
		return CasePage{}, invalidInput("page", "页码必须是正整数")
	}
	if query.PageSize < 1 || query.PageSize > 100 {
		return CasePage{}, invalidInput("pageSize", "每页数量必须在 1 至 100 之间")
	}
	if query.Status != "" && !domain.ValidStatus(query.Status) {
		return CasePage{}, invalidInput("status", "未知档案状态")
	}
	return s.repository.Query(ctx, query)
}

func (s *Service) VerifyStore(ctx context.Context) error { return s.repository.Verify(ctx) }

func (s *Service) CreateCase(ctx context.Context, command CreateCaseCommand) (OperationResult, error) {
	if err := validateMeta(command.CommandMeta, true); err != nil {
		return OperationResult{}, err
	}
	if command.ExpectedVersion != 0 {
		return OperationResult{}, domain.ErrVersionConflict
	}
	if err := domain.RequireRole(command.Role, domain.RoleConservator); err != nil {
		return OperationResult{}, err
	}
	payloadHash, err := domain.HashJSON(command)
	if err != nil {
		return OperationResult{}, err
	}
	scope := "cases:create:" + strings.TrimSpace(command.AccessionCode)
	if result, found, err := s.repository.LookupIdempotency(ctx, scope, command.IdempotencyKey, payloadHash); found || err != nil {
		return decodeResult(result, err)
	}
	now := s.now()
	c, err := domain.NewCase(s.newID("case"), command.AccessionCode, command.ShelfLocation, command.Title, command.ResponsibleConservator, now)
	if err != nil {
		return OperationResult{}, err
	}
	event := domain.NewEvent(s.newID("evt"), c.ID, "CASE_CREATED", command.Actor, command.Role, normalizeReason(command.Reason, "创建修复档案"), c.Version, now, map[string]any{"accessionCode": c.AccessionCode})
	audit.Append(c, event)
	return s.commit(ctx, scope, payloadHash, command.CommandMeta, c, event, nil)
}

func (s *Service) AddDamage(ctx context.Context, caseID string, command AddDamageCommand) (OperationResult, error) {
	return s.mutate(ctx, caseID, "damage:add", command.CommandMeta, command, []domain.Role{domain.RoleConservator}, func(c *domain.ConservationCase, now time.Time) (domain.Event, *domain.ReleaseCredential, error) {
		inputs := command.Records
		if len(inputs) == 0 {
			inputs = []domain.DamageInput{{FolioRef: command.FolioRef, DamageType: command.DamageType, Extent: command.Extent, Severity: command.Severity, EvidenceNote: command.EvidenceNote}}
		}
		batch := make([]domain.DamageObservation, 0, len(inputs))
		for _, input := range inputs {
			damage, err := domain.NewDamage(s.newID("dmg"), c.ID, input.FolioRef, input.DamageType, input.Extent, input.Severity, input.EvidenceNote, command.Actor, now)
			if err != nil {
				return domain.Event{}, nil, err
			}
			batch = append(batch, damage)
		}
		if err := c.AddDamageBatch(batch, now); err != nil {
			return domain.Event{}, nil, err
		}
		return domain.NewEvent(s.newID("evt"), c.ID, "DAMAGE_BASELINE_BATCH_RECORDED", command.Actor, command.Role, normalizeReason(command.Reason, "批量登记损伤基线"), c.Version, now, map[string]any{"batchCount": len(batch), "records": batch, "summary": c.DamageSummary}), nil, nil
	})
}

func (s *Service) CreatePlan(ctx context.Context, caseID string, command CreatePlanCommand) (OperationResult, error) {
	return s.mutate(ctx, caseID, "plan:create", command.CommandMeta, command, []domain.Role{domain.RoleConservator}, func(c *domain.ConservationCase, now time.Time) (domain.Event, *domain.ReleaseCredential, error) {
		plan, err := domain.NewPlanWithOptions(s.newID("plan"), c, command.Steps, command.Materials, command.PaperConstraint, command.PigmentConstraint, command.BindingConstraint, command.ChangeReason, command.RequiredSampleRounds, command.RemediationResolutions, command.Actor, now)
		if err != nil {
			return domain.Event{}, nil, err
		}
		if err := c.AddPlan(plan, now); err != nil {
			return domain.Event{}, nil, err
		}
		return domain.NewEvent(s.newID("evt"), c.ID, "PLAN_REVISION_CREATED", command.Actor, command.Role, normalizeReason(command.Reason, "编制修复方案"), c.Version, now, map[string]any{"planRevisionID": plan.ID, "revisionNumber": plan.RevisionNumber, "contentHash": plan.ContentHash, "coveredCount": len(plan.Coverage.CoveredDamageIDs), "uncoveredCount": len(plan.Coverage.UncoveredDamageIDs), "remediationCount": len(plan.RemediationResolutions)}), nil, nil
	})
}

func (s *Service) SubmitPlan(ctx context.Context, caseID string, command SubmitPlanCommand) (OperationResult, error) {
	return s.mutate(ctx, caseID, "plan:submit", command.CommandMeta, command, []domain.Role{domain.RoleConservator}, func(c *domain.ConservationCase, now time.Time) (domain.Event, *domain.ReleaseCredential, error) {
		if err := c.SubmitPlan(now); err != nil {
			return domain.Event{}, nil, err
		}
		return domain.NewEvent(s.newID("evt"), c.ID, "PLAN_SUBMITTED", command.Actor, command.Role, normalizeReason(command.Reason, "提交材料相容性核验"), c.Version, now, map[string]string{"planRevisionID": c.CurrentPlanRevisionID}), nil, nil
	})
}

func randomID(prefix string) string {
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		panic(fmt.Sprintf("系统随机源不可用: %v", err))
	}
	return prefix + "_" + hex.EncodeToString(bytes)
}
