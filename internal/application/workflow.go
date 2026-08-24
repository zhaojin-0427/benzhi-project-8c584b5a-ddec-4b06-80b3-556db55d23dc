package application

import (
	"context"
	"slices"
	"strings"
	"time"

	"manuscript-conservation-gate/internal/audit"
	"manuscript-conservation-gate/internal/domain"
)

func (s *Service) AssessCompatibility(ctx context.Context, caseID string, command AssessCommand) (OperationResult, error) {
	return s.mutate(ctx, caseID, "assessment:compatibility", command.CommandMeta, command, []domain.Role{domain.RoleVerifier}, func(c *domain.ConservationCase, now time.Time) (domain.Event, *domain.ReleaseCredential, error) {
		if len(command.WarningDispositions) > 0 {
			if err := c.ConfirmWarnings(command.WarningDispositions, command.Actor, now); err != nil {
				return domain.Event{}, nil, err
			}
			return domain.NewEvent(s.newID("evt"), c.ID, "COMPATIBILITY_WARNINGS_CONFIRMED", command.Actor, command.Role, normalizeReason(command.Reason, "确认相容性警示处置"), c.Version, now, map[string]any{"confirmedCount": len(command.WarningDispositions)}), nil, nil
		}
		plan, err := c.CurrentPlan()
		if err != nil {
			return domain.Event{}, nil, err
		}
		assessment, err := domain.EvaluateCompatibility(s.newID("asm"), *plan, command.Actor, now)
		if err != nil {
			return domain.Event{}, nil, err
		}
		if err := c.AddAssessment(assessment, now); err != nil {
			return domain.Event{}, nil, err
		}
		return domain.NewEvent(s.newID("evt"), c.ID, "COMPATIBILITY_ASSESSED", command.Actor, command.Role, normalizeReason(command.Reason, "执行材料相容性核验"), c.Version, now, assessment), nil, nil
	})
}

func (s *Service) RecordSample(ctx context.Context, caseID string, command RecordSampleCommand) (OperationResult, error) {
	return s.mutate(ctx, caseID, "assessment:sample", command.CommandMeta, command, []domain.Role{domain.RoleVerifier, domain.RoleConservator}, func(c *domain.ConservationCase, now time.Time) (domain.Event, *domain.ReleaseCredential, error) {
		round := domain.SampleRound{ID: s.newID("sample"), Round: command.Round, MaterialBatch: command.MaterialBatch, TemperatureC: command.TemperatureC, HumidityPercent: command.HumidityPercent, DurationMinutes: command.DurationMinutes, ColorDifference: command.ColorDifference, Deformation: command.Deformation, Observations: command.Observations, Outcome: command.SampleOutcome, PerformedBy: command.Actor}
		if round.MaterialBatch == "" && command.SampleConditions != "" {
			round.MaterialBatch = "旧请求未单列批次"
			round.TemperatureC = 22
			round.HumidityPercent = 50
			round.DurationMinutes = 60
			round.ColorDifference = "见观察说明"
			round.Deformation = "见观察说明"
			round.Observations = strings.TrimSpace(command.SampleConditions) + "；" + strings.TrimSpace(command.SampleObservations)
		}
		if err := c.AppendSampleRound(round, now); err != nil {
			return domain.Event{}, nil, err
		}
		assessment, _ := c.CurrentAssessment()
		saved := assessment.SampleRounds[len(assessment.SampleRounds)-1]
		roundEvent := domain.NewEvent(s.newID("evt"), c.ID, "SAMPLE_ROUND_RECORDED", command.Actor, command.Role, normalizeReason(command.Reason, "追加模拟试样轮次"), c.Version, now, saved)
		if c.Status == domain.StatusPendingReview || c.Status == domain.StatusReturned {
			audit.Append(c, roundEvent)
			eventType := "SAMPLE_ASSESSMENT_PASSED"
			fallback := "必需试样轮次全部通过"
			if c.Status == domain.StatusReturned {
				eventType = "SAMPLE_ASSESSMENT_FAILED"
				fallback = "试样失败并退回整改"
			}
			return domain.NewEvent(s.newID("evt"), c.ID, eventType, command.Actor, command.Role, normalizeReason(command.Reason, fallback), c.Version, now, map[string]any{"passedCount": assessment.SamplePassedCount, "failedCount": assessment.SampleFailedCount, "outcome": assessment.SampleOutcome}), nil, nil
		}
		return roundEvent, nil, nil
	})
}

func (s *Service) Review(ctx context.Context, caseID string, command ReviewCommand) (OperationResult, error) {
	return s.mutate(ctx, caseID, "review:decision", command.CommandMeta, command, []domain.Role{domain.RoleExpert}, func(c *domain.ConservationCase, now time.Time) (domain.Event, *domain.ReleaseCredential, error) {
		currentEvidence, err := c.BuildEvidenceSnapshot()
		if err != nil {
			return domain.Event{}, nil, err
		}
		snapshot := domain.EvidenceSnapshot{PlanRevisionID: command.PlanRevisionID, PlanContentHash: command.PlanContentHash, Digest: command.EvidenceDigest, ConfirmedItemIDs: command.ConfirmedEvidenceItemIDs}
		if snapshot.PlanRevisionID == "" && snapshot.PlanContentHash == "" && snapshot.Digest == "" {
			snapshot = currentEvidence
			snapshot.ConfirmedItemIDs = append([]string(nil), currentEvidence.ItemIDs...)
		}
		review, err := domain.NewReview(s.newID("rev"), c.CurrentPlanRevisionID, command.Decision, command.Comments, snapshot, command.Actor, now)
		if err != nil {
			return domain.Event{}, nil, err
		}
		if err := c.ApplyReview(review, now); err != nil {
			return domain.Event{}, nil, err
		}
		eventType := "REVIEW_APPROVED"
		fallback := "批准修复方案"
		if command.Decision == domain.ReviewReturned {
			eventType, fallback = "REVIEW_RETURNED", "附意见退回整改"
		}
		return domain.NewEvent(s.newID("evt"), c.ID, eventType, command.Actor, command.Role, normalizeReason(command.Reason, fallback), c.Version, now, review), nil, nil
	})
}

func (s *Service) Release(ctx context.Context, caseID string, command ReleaseCommand) (OperationResult, error) {
	return s.mutate(ctx, caseID, "release:issue", command.CommandMeta, command, []domain.Role{domain.RoleExpert}, func(c *domain.ConservationCase, now time.Time) (domain.Event, *domain.ReleaseCredential, error) {
		plan, err := c.CurrentPlan()
		if err != nil {
			return domain.Event{}, nil, err
		}
		evidence, err := c.GateEvidenceSnapshot()
		if err != nil {
			return domain.Event{}, nil, err
		}
		if command.PlanRevisionID != "" && (command.PlanRevisionID != plan.ID || command.PlanContentHash != plan.ContentHash || command.EvidenceDigest != evidence.Digest) {
			return domain.Event{}, nil, invalidInput("evidenceDigest", "放行确认与服务端当前获批方案或证据摘要不一致")
		}
		if err := c.Freeze(now); err != nil {
			return domain.Event{}, nil, err
		}
		event := domain.NewEvent(s.newID("evt"), c.ID, "PLAN_FROZEN_AND_RELEASED", command.Actor, command.Role, normalizeReason(command.Reason, "冻结获批方案并签发放行凭据"), c.Version, now, map[string]string{"planRevisionID": c.CurrentPlanRevisionID, "planContentHash": plan.ContentHash, "evidenceDigest": evidence.Digest})
		// 凭据必须绑定本次放行事件写入后的审计链头，审计记录由 mutate 追加后签发。
		return event, &domain.ReleaseCredential{}, nil
	})
}

func (s *Service) VerifyCredential(ctx context.Context, caseID string) (VerificationResult, error) {
	c, err := s.repository.Get(ctx, caseID)
	if err != nil {
		return VerificationResult{}, err
	}
	valid, err := s.issuer.Verify(c)
	if err != nil {
		return VerificationResult{}, err
	}
	message := "凭据签名、方案摘要与审计链均有效"
	if !valid {
		message = "凭据校验失败，内容可能已被更改"
	}
	checks := VerificationChecks{}
	if c.Credential != nil {
		plan, planErr := c.CurrentPlan()
		if planErr == nil {
			checks.PlanDigest = c.Credential.ContentDigest == domain.PlanDigest(*plan)
			checks.Frozen = plan.Frozen && c.Status == domain.StatusReleased
		}
		evidence, evidenceErr := c.GateEvidenceSnapshot()
		checks.EvidenceDigest = evidenceErr == nil && evidence.Digest == c.Credential.EvidenceDigest && slices.Equal(evidence.ItemIDs, c.Credential.EvidenceItemIDs)
		checks.AuditHead = len(c.AuditTrail) > 0 && c.Credential.AuditHeadHash == c.AuditHeadHash && audit.Verify(c.AuditTrail) == nil
		checks.Signature = s.issuer.VerifySignature(*c.Credential)
	}
	overall := checks.PlanDigest && checks.Frozen && checks.EvidenceDigest && checks.AuditHead && checks.Signature
	if !overall {
		message = "凭据分项校验未全部通过"
	}
	return VerificationResult{Valid: overall, Credential: c.Credential, Message: message, Checks: checks}, nil
}
