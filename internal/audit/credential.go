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
	key := signatureCacheKey(credential)
	if verified, ok := i.verifiedSignatures.Load(key); ok {
		return verified.(bool)
	}
	verified := domain.VerifyCredentialSignature(credential, i.secret)
	i.verifiedSignatures.Store(key, verified)
	return verified
}

// signatureCacheKey 覆盖参与凭据签名计算的全部输入。凭据 ID 在同一档案放行流程
// 内保持稳定，但签名是否合法取决于 ID、方案摘要、证据摘要、审计链头、批准人、
// 签发时间与凭据声明的 Signature 共同决定。仓储刷新出 ID 相同但签名输入已变化
// 的凭据时，组合键不再命中陈旧缓存，使签名校验重新执行并判定为无效；未变化的
// 合法凭据各字段不变，仍命中缓存。
func signatureCacheKey(c domain.ReleaseCredential) string {
	return c.ID + "\x1f" + c.CaseID + "\x1f" + c.FrozenPlanRevisionID + "\x1f" + c.ApprovedBy + "\x1f" + c.IssuedAt.UTC().Format(time.RFC3339Nano) + "\x1f" + c.ContentDigest + "\x1f" + c.EvidenceDigest + "\x1f" + c.AuditHeadHash + "\x1f" + c.Signature
}
