package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

type TreatmentStep struct {
	Order       int      `json:"order"`
	Description string   `json:"description"`
	Technique   string   `json:"technique"`
	DamageIDs   []string `json:"damageIDs"`
}

type Material struct {
	Name          string  `json:"name"`
	Category      string  `json:"category"`
	PH            float64 `json:"ph"`
	Reversible    bool    `json:"reversible"`
	ContainsMetal bool    `json:"containsMetal"`
	WaterBased    bool    `json:"waterBased"`
}

type TreatmentPlanRevision struct {
	ID                     string                  `json:"id"`
	CaseID                 string                  `json:"caseID"`
	RevisionNumber         int                     `json:"revisionNumber"`
	Steps                  []TreatmentStep         `json:"steps"`
	Materials              []Material              `json:"materials"`
	PaperConstraint        string                  `json:"paperConstraint"`
	PigmentConstraint      string                  `json:"pigmentConstraint"`
	BindingConstraint      string                  `json:"bindingConstraint"`
	ChangeReason           string                  `json:"changeReason"`
	RequiredSampleRounds   int                     `json:"requiredSampleRounds"`
	Coverage               DamageCoverage          `json:"coverage"`
	RemediationResolutions []RemediationResolution `json:"remediationResolutions,omitempty"`
	ContentHash            string                  `json:"contentHash"`
	SemanticHash           string                  `json:"semanticHash"`
	CreatedAt              time.Time               `json:"createdAt"`
	SubmittedAt            *time.Time              `json:"submittedAt,omitempty"`
	SupersedesRevisionID   string                  `json:"supersedesRevisionID,omitempty"`
	Frozen                 bool                    `json:"frozen"`
}

func (p TreatmentPlanRevision) Clone() TreatmentPlanRevision {
	p.Steps = append([]TreatmentStep(nil), p.Steps...)
	for i := range p.Steps {
		p.Steps[i].DamageIDs = append([]string(nil), p.Steps[i].DamageIDs...)
	}
	p.Materials = append([]Material(nil), p.Materials...)
	p.Coverage = p.Coverage.Clone()
	p.RemediationResolutions = append([]RemediationResolution(nil), p.RemediationResolutions...)
	p.SubmittedAt = cloneTimePtr(p.SubmittedAt)
	return p
}

func cloneTimePtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	c := *t
	return &c
}

type DamageCoverage struct {
	CoveredDamageIDs       []string `json:"coveredDamageIDs"`
	UncoveredDamageIDs     []string `json:"uncoveredDamageIDs"`
	UncoveredHighDamageIDs []string `json:"uncoveredHighDamageIDs"`
}

func (c DamageCoverage) Clone() DamageCoverage {
	c.CoveredDamageIDs = append([]string(nil), c.CoveredDamageIDs...)
	c.UncoveredDamageIDs = append([]string(nil), c.UncoveredDamageIDs...)
	c.UncoveredHighDamageIDs = append([]string(nil), c.UncoveredHighDamageIDs...)
	return c
}

type RemediationResolution struct {
	ItemID       string `json:"itemID"`
	StepOrder    int    `json:"stepOrder,omitempty"`
	MaterialName string `json:"materialName,omitempty"`
	Description  string `json:"description"`
	ResolvedBy   string `json:"resolvedBy,omitempty"`
}

type RemediationItem struct {
	ID                     string     `json:"id"`
	PlanRevisionID         string     `json:"planRevisionID"`
	Source                 string     `json:"source"`
	SourceID               string     `json:"sourceID"`
	FolioRef               string     `json:"folioRef,omitempty"`
	Description            string     `json:"description"`
	Required               bool       `json:"required"`
	Closed                 bool       `json:"closed"`
	ClosedByPlanRevisionID string     `json:"closedByPlanRevisionID,omitempty"`
	ClosedBy               string     `json:"closedBy,omitempty"`
	ClosedAt               *time.Time `json:"closedAt,omitempty"`
}

func NewPlan(id string, c *ConservationCase, steps []TreatmentStep, materials []Material, paper, pigment, binding, reason string, now time.Time) (TreatmentPlanRevision, error) {
	if len(steps) > 0 {
		hasRefs := false
		for _, s := range steps {
			if len(s.DamageIDs) > 0 {
				hasRefs = true
			}
		}
		if !hasRefs {
			for _, d := range c.Damages {
				if d.Severity == SeverityHigh {
					steps[0].DamageIDs = append(steps[0].DamageIDs, d.ID)
				}
			}
		}
	}
	return NewPlanWithOptions(id, c, steps, materials, paper, pigment, binding, reason, 1, nil, "", now)
}

func NewPlanWithOptions(id string, c *ConservationCase, steps []TreatmentStep, materials []Material, paper, pigment, binding, reason string, requiredRounds int, resolutions []RemediationResolution, actor string, now time.Time) (TreatmentPlanRevision, error) {
	p := TreatmentPlanRevision{ID: id, CaseID: c.ID, RevisionNumber: len(c.PlanRevisions) + 1, Steps: append([]TreatmentStep(nil), steps...), Materials: append([]Material(nil), materials...),
		PaperConstraint: strings.TrimSpace(paper), PigmentConstraint: strings.TrimSpace(pigment), BindingConstraint: strings.TrimSpace(binding), ChangeReason: strings.TrimSpace(reason), RequiredSampleRounds: requiredRounds, RemediationResolutions: append([]RemediationResolution(nil), resolutions...), CreatedAt: now, SupersedesRevisionID: c.CurrentPlanRevisionID}
	if p.RequiredSampleRounds == 0 {
		p.RequiredSampleRounds = 1
	}
	if p.RequiredSampleRounds < 1 || p.RequiredSampleRounds > 10 {
		return p, invalid("requiredSampleRounds", "必需试样轮数应为 1 至 10")
	}
	if len(c.Damages) == 0 {
		return p, invalid("damages", "至少登记一条损伤基线后才能编制方案")
	}
	if c.Status != StatusDraft && c.Status != StatusReturned {
		return p, ErrInvalidTransition
	}
	if len(p.Steps) == 0 {
		return p, invalid("steps", "方案至少包含一个修复步骤")
	}
	for i := range p.Steps {
		p.Steps[i].Order = i + 1
		p.Steps[i].Description = strings.TrimSpace(p.Steps[i].Description)
		p.Steps[i].Technique = strings.TrimSpace(p.Steps[i].Technique)
		if p.Steps[i].Description == "" || p.Steps[i].Technique == "" {
			return p, invalid("steps", "每个步骤必须填写说明与工艺")
		}
		seenRefs := map[string]struct{}{}
		for j, id := range p.Steps[i].DamageIDs {
			id = strings.TrimSpace(id)
			p.Steps[i].DamageIDs[j] = id
			if _, ok := seenRefs[id]; ok {
				return p, invalid("steps.damageIDs", "同一步骤不得重复关联损伤")
			}
			seenRefs[id] = struct{}{}
		}
	}
	if len(p.Materials) == 0 {
		return p, invalid("materials", "方案至少包含一种材料")
	}
	for i := range p.Materials {
		p.Materials[i].Name = strings.TrimSpace(p.Materials[i].Name)
		p.Materials[i].Category = strings.TrimSpace(p.Materials[i].Category)
		if p.Materials[i].Name == "" || p.Materials[i].Category == "" {
			return p, invalid("materials", "材料名称和类别不能为空")
		}
		if p.Materials[i].PH < 0 || p.Materials[i].PH > 14 {
			return p, invalid("materials.ph", "材料 pH 必须在 0 至 14 之间")
		}
	}
	if p.RevisionNumber > 1 && len(p.ChangeReason) < 3 {
		return p, invalid("changeReason", "修订方案必须说明变更原因")
	}
	coverage, err := calculateCoverage(c, p.Steps)
	if err != nil {
		return p, err
	}
	p.Coverage = coverage
	if len(coverage.UncoveredHighDamageIDs) > 0 {
		return p, invalid("steps.damageIDs", "严重损伤未被方案步骤覆盖："+strings.Join(damageLabels(c, coverage.UncoveredHighDamageIDs), "、"))
	}
	if c.Status == StatusReturned {
		if err := applyRemediation(c, &p, actor, now); err != nil {
			return p, err
		}
	}
	p.SemanticHash = PlanSemanticDigest(p)
	if p.RevisionNumber > 1 {
		old, _ := c.CurrentPlan()
		if old != nil && old.SemanticHash == p.SemanticHash {
			return p, invalid("plan", "整改方案与上一版本的规范化内容没有变化")
		}
	}
	p.ContentHash = PlanDigest(p)
	return p, nil
}

func PlanDigest(p TreatmentPlanRevision) string {
	canonical := struct {
		Revision       int                     `json:"revision"`
		Steps          []TreatmentStep         `json:"steps"`
		Materials      []Material              `json:"materials"`
		Paper          string                  `json:"paper"`
		Pigment        string                  `json:"pigment"`
		Binding        string                  `json:"binding"`
		RequiredRounds int                     `json:"requiredSampleRounds"`
		Coverage       DamageCoverage          `json:"coverage"`
		Resolutions    []RemediationResolution `json:"remediationResolutions,omitempty"`
	}{p.RevisionNumber, p.Steps, p.Materials, p.PaperConstraint, p.PigmentConstraint, p.BindingConstraint, p.RequiredSampleRounds, p.Coverage, p.RemediationResolutions}
	b, _ := json.Marshal(canonical)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func PlanSemanticDigest(p TreatmentPlanRevision) string {
	p.RevisionNumber = 0
	p.ID = ""
	p.ContentHash = ""
	p.SemanticHash = ""
	p.CreatedAt = time.Time{}
	p.SubmittedAt = nil
	p.SupersedesRevisionID = ""
	p.Frozen = false
	p.RemediationResolutions = nil
	b, _ := json.Marshal(p)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func calculateCoverage(c *ConservationCase, steps []TreatmentStep) (DamageCoverage, error) {
	known := map[string]DamageObservation{}
	for _, d := range c.Damages {
		known[d.ID] = d
	}
	covered := map[string]struct{}{}
	for _, step := range steps {
		for _, id := range step.DamageIDs {
			d, ok := known[id]
			if !ok || d.CaseID != c.ID {
				return DamageCoverage{}, invalid("steps.damageIDs", "方案步骤引用了未知或其他档案的损伤记录："+id)
			}
			covered[id] = struct{}{}
		}
	}
	result := DamageCoverage{}
	for _, d := range c.Damages {
		if _, ok := covered[d.ID]; ok {
			result.CoveredDamageIDs = append(result.CoveredDamageIDs, d.ID)
		} else {
			result.UncoveredDamageIDs = append(result.UncoveredDamageIDs, d.ID)
			if d.Severity == SeverityHigh {
				result.UncoveredHighDamageIDs = append(result.UncoveredHighDamageIDs, d.ID)
			}
		}
	}
	return result, nil
}

func damageLabels(c *ConservationCase, ids []string) []string {
	set := map[string]struct{}{}
	for _, id := range ids {
		set[id] = struct{}{}
	}
	out := []string{}
	for _, d := range c.Damages {
		if _, ok := set[d.ID]; ok {
			out = append(out, d.FolioRef+"/"+d.DamageType)
		}
	}
	return out
}

func applyRemediation(c *ConservationCase, p *TreatmentPlanRevision, actor string, now time.Time) error {
	open := map[string]*RemediationItem{}
	for i := range c.RemediationItems {
		if c.RemediationItems[i].Required && !c.RemediationItems[i].Closed {
			open[c.RemediationItems[i].ID] = &c.RemediationItems[i]
		}
	}
	provided := map[string]struct{}{}
	for i := range p.RemediationResolutions {
		r := &p.RemediationResolutions[i]
		r.ItemID = strings.TrimSpace(r.ItemID)
		r.MaterialName = strings.TrimSpace(r.MaterialName)
		r.Description = strings.TrimSpace(r.Description)
		item, ok := open[r.ItemID]
		if !ok {
			return invalid("remediationResolutions", "整改处置引用了其他档案、未知或已失效的整改项："+r.ItemID)
		}
		if _, dup := provided[r.ItemID]; dup {
			return invalid("remediationResolutions", "同一整改项不得重复处置")
		}
		provided[r.ItemID] = struct{}{}
		if len(r.Description) < 3 {
			return invalid("remediationResolutions.description", "整改处置说明至少 3 个字符")
		}
		if r.StepOrder == 0 && r.MaterialName == "" {
			return invalid("remediationResolutions", "整改项必须关联具体步骤或材料变更")
		}
		if r.StepOrder < 0 || r.StepOrder > len(p.Steps) {
			return invalid("remediationResolutions.stepOrder", "整改项关联了不存在的方案步骤")
		}
		if r.MaterialName != "" {
			found := false
			for _, m := range p.Materials {
				if m.Name == r.MaterialName {
					found = true
				}
			}
			if !found {
				return invalid("remediationResolutions.materialName", "整改项关联了不存在的材料")
			}
		}
		r.ResolvedBy = strings.TrimSpace(actor)
		item.Closed = true
		item.ClosedByPlanRevisionID = p.ID
		item.ClosedBy = r.ResolvedBy
		item.ClosedAt = &now
	}
	missing := []string{}
	for id, item := range open {
		if _, ok := provided[id]; !ok {
			missing = append(missing, item.Source+"："+item.Description)
		}
	}
	if len(missing) > 0 {
		return invalid("remediationResolutions", "仍有必改项未处置："+strings.Join(missing, "；"))
	}
	return nil
}

func (c *ConservationCase) AddPlan(p TreatmentPlanRevision, now time.Time) error {
	if c.Status != StatusDraft && c.Status != StatusReturned {
		return ErrInvalidTransition
	}
	c.PlanRevisions = append(c.PlanRevisions, p)
	c.CurrentPlanRevisionID = p.ID
	c.Advance(StatusDraft, now)
	return nil
}

func (c *ConservationCase) SubmitPlan(now time.Time) error {
	if c.Status != StatusDraft {
		return ErrInvalidTransition
	}
	p, err := c.CurrentPlan()
	if err != nil {
		return err
	}
	if p.SubmittedAt != nil {
		return invalid("planRevisionID", "该方案版本已经提交")
	}
	coverage, err := calculateCoverage(c, p.Steps)
	if err != nil {
		return err
	}
	if len(coverage.UncoveredHighDamageIDs) > 0 {
		return invalid("coverage", "严重损伤覆盖门禁未通过："+strings.Join(damageLabels(c, coverage.UncoveredHighDamageIDs), "、"))
	}
	p.Coverage = coverage
	p.SemanticHash = PlanSemanticDigest(*p)
	p.ContentHash = PlanDigest(*p)
	for _, item := range c.RemediationItems {
		if item.Required && !item.Closed {
			return invalid("remediationItems", "仍有必改项未闭环："+item.Source+" / "+item.Description)
		}
	}
	p.SubmittedAt = &now
	c.Advance(StatusPendingCompatibility, now)
	return nil
}
