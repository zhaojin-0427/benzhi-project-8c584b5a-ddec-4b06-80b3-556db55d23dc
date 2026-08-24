package audit

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"manuscript-conservation-gate/internal/domain"
)

type Issuer struct {
	secret             string
	verifiedSignatures sync.Map
}

func NewIssuer(secret string) (*Issuer, error) {
	secret = strings.TrimSpace(secret)
	if len(secret) < 16 {
		return nil, fmt.Errorf("凭据签发密钥至少需要 16 个字符")
	}
	return &Issuer{secret: secret}, nil
}

func (i *Issuer) Issue(id string, c *domain.ConservationCase, approver string, now time.Time) (domain.ReleaseCredential, error) {
	plan, err := c.CurrentPlan()
	if err != nil {
		return domain.ReleaseCredential{}, err
	}
	if !plan.Frozen || c.Status != domain.StatusReleased {
		return domain.ReleaseCredential{}, domain.ErrInvalidTransition
	}
	evidence, err := c.GateEvidenceSnapshot()
	if err != nil {
		return domain.ReleaseCredential{}, err
	}
	credential := domain.ReleaseCredential{ID: id, CaseID: c.ID, FrozenPlanRevisionID: plan.ID,
		ApprovedBy: strings.TrimSpace(approver), IssuedAt: now.UTC(), ContentDigest: plan.ContentHash, EvidenceDigest: evidence.Digest, EvidenceItemIDs: append([]string(nil), evidence.ItemIDs...), AuditHeadHash: c.AuditHeadHash}
	if credential.ApprovedBy == "" {
		return credential, fmt.Errorf("%w: 批准人不能为空", domain.ErrValidation)
	}
	credential.Signature = domain.CredentialSignature(credential.ID, credential.CaseID, credential.FrozenPlanRevisionID,
		credential.ApprovedBy, credential.IssuedAt, credential.ContentDigest, credential.EvidenceDigest, credential.AuditHeadHash, i.secret)
	return credential, nil
}

func (i *Issuer) Verify(c *domain.ConservationCase) (bool, error) {
	if c.Credential == nil {
		return false, fmt.Errorf("%w: 档案尚未签发凭据", domain.ErrValidation)
	}
	plan, err := c.CurrentPlan()
	if err != nil {
		return false, err
	}
	if err := Verify(c.AuditTrail); err != nil {
		return false, err
	}
	if c.Credential.AuditHeadHash != c.AuditHeadHash {
		return false, nil
	}
	evidence, err := c.GateEvidenceSnapshot()
	if err != nil {
		return false, err
	}
	if evidence.Digest != c.Credential.EvidenceDigest || !slices.Equal(evidence.ItemIDs, c.Credential.EvidenceItemIDs) {
		return false, nil
	}
	return domain.VerifyCredential(*c.Credential, *plan, i.secret), nil
}

func (i *Issuer) VerifySignature(credential domain.ReleaseCredential) bool {
	if verified, ok := i.verifiedSignatures.Load(credential.ID); ok {
		return verified.(bool)
	}
	verified := domain.VerifyCredentialSignature(credential, i.secret)
	i.verifiedSignatures.Store(credential.ID, verified)
	return verified
}
