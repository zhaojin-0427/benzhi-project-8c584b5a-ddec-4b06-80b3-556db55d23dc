package domain

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
	"time"
)

type ReleaseCredential struct {
	ID                   string    `json:"id"`
	CaseID               string    `json:"caseID"`
	FrozenPlanRevisionID string    `json:"frozenPlanRevisionID"`
	ApprovedBy           string    `json:"approvedBy"`
	IssuedAt             time.Time `json:"issuedAt"`
	ContentDigest        string    `json:"contentDigest"`
	EvidenceDigest       string    `json:"evidenceDigest"`
	EvidenceItemIDs      []string  `json:"evidenceItemIDs"`
	AuditHeadHash        string    `json:"auditHeadHash"`
	Signature            string    `json:"signature"`
}

func (c *ConservationCase) ValidateReleaseGate() error {
	if c.Status != StatusApproved {
		return ErrInvalidTransition
	}
	plan, err := c.CurrentPlan()
	if err != nil {
		return err
	}
	if plan.ContentHash != PlanDigest(*plan) {
		return invalid("contentHash", "方案摘要与内容不一致")
	}
	a, err := c.currentAssessment()
	if err != nil {
		return err
	}
	if a.BlockingCount > 0 {
		return invalid("assessment", "仍存在材料相容性阻断项")
	}
	if a.UnconfirmedWarningCount > 0 {
		return invalid("warningDispositions", "仍有相容性警示未确认")
	}
	if a.SampleOutcome != SamplePass {
		return invalid("sampleOutcome", "模拟试样尚未通过")
	}
	if !c.HasApprovedCurrentPlan() {
		return invalid("review", "当前方案尚未获得专家批准")
	}
	for _, item := range c.RemediationItems {
		if item.Required && !item.Closed {
			return invalid("remediationItems", "仍有整改项未闭环："+item.ID)
		}
	}
	return nil
}

func (c *ConservationCase) Freeze(now time.Time) error {
	if err := c.ValidateReleaseGate(); err != nil {
		return err
	}
	plan, _ := c.CurrentPlan()
	plan.Frozen = true
	c.Advance(StatusReleased, now)
	return nil
}

func CredentialSignature(id, caseID, planID, approver string, issued time.Time, digest, evidenceDigest, auditHead, secret string) string {
	plain := strings.Join([]string{id, caseID, planID, approver, issued.UTC().Format(time.RFC3339Nano), digest, evidenceDigest, auditHead, secret}, "\x1f")
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

func VerifyCredential(credential ReleaseCredential, plan TreatmentPlanRevision, secret string) bool {
	if credential.ContentDigest != PlanDigest(plan) || credential.FrozenPlanRevisionID != plan.ID || !plan.Frozen {
		return false
	}
	return VerifyCredentialSignature(credential, secret)
}

func VerifyCredentialSignature(credential ReleaseCredential, secret string) bool {
	expected := CredentialSignature(credential.ID, credential.CaseID, credential.FrozenPlanRevisionID, credential.ApprovedBy, credential.IssuedAt, credential.ContentDigest, credential.EvidenceDigest, credential.AuditHeadHash, secret)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(credential.Signature)) == 1
}
