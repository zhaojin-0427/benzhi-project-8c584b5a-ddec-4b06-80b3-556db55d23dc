package domain

import (
	"strings"
	"time"
)

type ConservationCase struct {
	ID                     string                    `json:"id"`
	AccessionCode          string                    `json:"accessionCode"`
	ShelfLocation          string                    `json:"shelfLocation"`
	Title                  string                    `json:"title"`
	ResponsibleConservator string                    `json:"responsibleConservator"`
	Status                 Status                    `json:"status"`
	CurrentPlanRevisionID  string                    `json:"currentPlanRevisionID,omitempty"`
	Version                int64                     `json:"version"`
	CreatedAt              time.Time                 `json:"createdAt"`
	UpdatedAt              time.Time                 `json:"updatedAt"`
	Damages                []DamageObservation       `json:"damages"`
	DamageSummary          DamageBaselineSummary     `json:"damageSummary"`
	PlanRevisions          []TreatmentPlanRevision   `json:"planRevisions"`
	Assessments            []CompatibilityAssessment `json:"assessments"`
	Reviews                []ExpertReview            `json:"reviews"`
	RemediationItems       []RemediationItem         `json:"remediationItems"`
	AuditTrail             []AuditRecord             `json:"auditTrail"`
	AuditHeadHash          string                    `json:"auditHeadHash,omitempty"`
	Credential             *ReleaseCredential        `json:"credential,omitempty"`
}

func NewCase(id, accession, shelf, title, conservator string, now time.Time) (*ConservationCase, error) {
	accession = strings.TrimSpace(accession)
	shelf = strings.TrimSpace(shelf)
	title = strings.TrimSpace(title)
	conservator = strings.TrimSpace(conservator)
	if id == "" {
		return nil, invalid("id", "档案标识不能为空")
	}
	if len(accession) < 3 || len(accession) > 64 {
		return nil, invalid("accessionCode", "馆藏编号长度应为 3 至 64 个字符")
	}
	if len(shelf) < 2 || len(shelf) > 100 {
		return nil, invalid("shelfLocation", "馆藏定位长度应为 2 至 100 个字符")
	}
	if len(title) < 1 || len(title) > 120 {
		return nil, invalid("title", "题名长度应为 1 至 120 个字符")
	}
	if len(conservator) < 2 || len(conservator) > 80 {
		return nil, invalid("responsibleConservator", "责任修复师长度应为 2 至 80 个字符")
	}
	return &ConservationCase{ID: id, AccessionCode: accession, ShelfLocation: shelf, Title: title,
		ResponsibleConservator: conservator, Status: StatusDraft, Version: 1, CreatedAt: now, UpdatedAt: now,
		Damages: []DamageObservation{}, DamageSummary: NewDamageSummary(nil), PlanRevisions: []TreatmentPlanRevision{}, Assessments: []CompatibilityAssessment{}, Reviews: []ExpertReview{}, RemediationItems: []RemediationItem{}, AuditTrail: []AuditRecord{}}, nil
}

func (c *ConservationCase) Clone() *ConservationCase {
	copyCase := *c
	copyCase.Damages = append([]DamageObservation(nil), c.Damages...)
	copyCase.DamageSummary = DamageBaselineSummary{Total: c.DamageSummary.Total, ByFolio: map[string]int{}, ByType: map[string]int{}, BySeverity: map[Severity]int{}}
	for key, value := range c.DamageSummary.ByFolio {
		copyCase.DamageSummary.ByFolio[key] = value
	}
	for key, value := range c.DamageSummary.ByType {
		copyCase.DamageSummary.ByType[key] = value
	}
	for key, value := range c.DamageSummary.BySeverity {
		copyCase.DamageSummary.BySeverity[key] = value
	}
	copyCase.PlanRevisions = make([]TreatmentPlanRevision, len(c.PlanRevisions))
	for i := range c.PlanRevisions {
		copyCase.PlanRevisions[i] = c.PlanRevisions[i].Clone()
	}
	copyCase.Assessments = make([]CompatibilityAssessment, len(c.Assessments))
	for i := range c.Assessments {
		copyCase.Assessments[i] = c.Assessments[i].Clone()
	}
	copyCase.Reviews = make([]ExpertReview, len(c.Reviews))
	for i := range c.Reviews {
		copyCase.Reviews[i] = c.Reviews[i]
		copyCase.Reviews[i].Comments = append([]ReviewComment(nil), c.Reviews[i].Comments...)
		copyCase.Reviews[i].EvidenceSnapshot = c.Reviews[i].EvidenceSnapshot.Clone()
	}
	copyCase.RemediationItems = append([]RemediationItem(nil), c.RemediationItems...)
	for i := range copyCase.RemediationItems {
		copyCase.RemediationItems[i].ClosedAt = cloneTimePtr(copyCase.RemediationItems[i].ClosedAt)
	}
	copyCase.AuditTrail = append([]AuditRecord(nil), c.AuditTrail...)
	if c.Credential != nil {
		cred := *c.Credential
		cred.EvidenceItemIDs = append([]string(nil), c.Credential.EvidenceItemIDs...)
		copyCase.Credential = &cred
	}
	return &copyCase
}

func (c *ConservationCase) CurrentPlan() (*TreatmentPlanRevision, error) {
	for i := range c.PlanRevisions {
		if c.PlanRevisions[i].ID == c.CurrentPlanRevisionID {
			return &c.PlanRevisions[i], nil
		}
	}
	return nil, invalid("currentPlanRevisionID", "档案尚无当前修复方案")
}

func (c *ConservationCase) Advance(status Status, now time.Time) {
	c.Status = status
	c.Version++
	c.UpdatedAt = now
}

func (c *ConservationCase) EnsureMutable() error {
	if c.Status == StatusReleased {
		return ErrInvalidTransition
	}
	return nil
}
