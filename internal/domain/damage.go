package domain

import (
	"fmt"
	"strings"
	"time"
)

type DamageObservation struct {
	ID           string    `json:"id"`
	CaseID       string    `json:"caseID"`
	FolioRef     string    `json:"folioRef"`
	DamageType   string    `json:"damageType"`
	Extent       string    `json:"extent"`
	Severity     Severity  `json:"severity"`
	EvidenceNote string    `json:"evidenceNote"`
	RecordedBy   string    `json:"recordedBy"`
	RecordedAt   time.Time `json:"recordedAt"`
}

type DamageInput struct {
	FolioRef     string   `json:"folioRef"`
	DamageType   string   `json:"damageType"`
	Extent       string   `json:"extent"`
	Severity     Severity `json:"severity"`
	EvidenceNote string   `json:"evidenceNote"`
}

type DamageBaselineSummary struct {
	Total      int              `json:"total"`
	ByFolio    map[string]int   `json:"byFolio"`
	ByType     map[string]int   `json:"byDamageType"`
	BySeverity map[Severity]int `json:"bySeverity"`
}

func NewDamageSummary(items []DamageObservation) DamageBaselineSummary {
	s := DamageBaselineSummary{ByFolio: map[string]int{}, ByType: map[string]int{}, BySeverity: map[Severity]int{}}
	for _, d := range items {
		s.Total++
		s.ByFolio[d.FolioRef]++
		s.ByType[d.DamageType]++
		s.BySeverity[d.Severity]++
	}
	return s
}

func NewDamage(id, caseID, folio, damageType, extent string, severity Severity, evidence, actor string, now time.Time) (DamageObservation, error) {
	d := DamageObservation{ID: id, CaseID: caseID, FolioRef: strings.TrimSpace(folio), DamageType: strings.TrimSpace(damageType),
		Extent: strings.TrimSpace(extent), Severity: severity, EvidenceNote: strings.TrimSpace(evidence), RecordedBy: strings.TrimSpace(actor), RecordedAt: now}
	if d.FolioRef == "" {
		return d, invalid("folioRef", "页位不能为空")
	}
	if d.DamageType == "" {
		return d, invalid("damageType", "损伤类型不能为空")
	}
	if d.Extent == "" {
		return d, invalid("extent", "损伤范围不能为空")
	}
	if d.Severity != SeverityLow && d.Severity != SeverityModerate && d.Severity != SeverityHigh {
		return d, invalid("severity", "严重度取值无效")
	}
	if len(d.EvidenceNote) < 3 {
		return d, invalid("evidenceNote", "图文证据摘要至少 3 个字符")
	}
	if d.RecordedBy == "" {
		return d, invalid("actor", "记录人不能为空")
	}
	return d, nil
}

func (c *ConservationCase) AddDamage(d DamageObservation, now time.Time) error {
	return c.AddDamageBatch([]DamageObservation{d}, now)
}

func (c *ConservationCase) AddDamageBatch(batch []DamageObservation, now time.Time) error {
	if c.Status != StatusDraft && c.Status != StatusReturned {
		return ErrInvalidTransition
	}
	if len(batch) == 0 || len(batch) > 100 {
		return invalid("records", "损伤批次应包含 1 至 100 条记录")
	}
	seen := map[string]struct{}{}
	for _, existing := range c.Damages {
		seen[existing.FolioRef+"\x1f"+existing.DamageType] = struct{}{}
	}
	for i, d := range batch {
		if d.CaseID != c.ID {
			return invalid("records", "损伤记录引用了其他档案")
		}
		key := d.FolioRef + "\x1f" + d.DamageType
		if _, ok := seen[key]; ok {
			return invalid("records", fmt.Sprintf("第 %d 条与既有基线或本批次重复：%s / %s", i+1, d.FolioRef, d.DamageType))
		}
		seen[key] = struct{}{}
	}
	c.Damages = append(c.Damages, batch...)
	c.DamageSummary = NewDamageSummary(c.Damages)
	c.Advance(c.Status, now)
	return nil
}
