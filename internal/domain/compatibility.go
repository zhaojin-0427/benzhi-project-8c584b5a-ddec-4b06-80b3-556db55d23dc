package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

type FindingLevel string

const (
	FindingBlock   FindingLevel = "BLOCK"
	FindingWarning FindingLevel = "WARNING"
	FindingInfo    FindingLevel = "INFO"
)

type RuleFinding struct {
	ID         string       `json:"id"`
	Code       string       `json:"code"`
	Level      FindingLevel `json:"level"`
	Material   string       `json:"material,omitempty"`
	Message    string       `json:"message"`
	Resolution string       `json:"resolution,omitempty"`
}

type WarningDisposition struct {
	FindingID       string    `json:"findingID"`
	Method          string    `json:"method"`
	ControlMeasures string    `json:"controlMeasures"`
	ConfirmedBy     string    `json:"confirmedBy"`
	ConfirmedAt     time.Time `json:"confirmedAt"`
}

type SampleRound struct {
	ID              string        `json:"id"`
	Round           int           `json:"round"`
	MaterialBatch   string        `json:"materialBatch"`
	TemperatureC    float64       `json:"temperatureC"`
	HumidityPercent float64       `json:"humidityPercent"`
	DurationMinutes int           `json:"durationMinutes"`
	ColorDifference string        `json:"colorDifference"`
	Deformation     string        `json:"deformation"`
	Observations    string        `json:"observations"`
	Outcome         SampleOutcome `json:"outcome"`
	PerformedBy     string        `json:"performedBy"`
	PerformedAt     time.Time     `json:"performedAt"`
}

type CompatibilityAssessment struct {
	ID                      string               `json:"id"`
	PlanRevisionID          string               `json:"planRevisionID"`
	RuleFindings            []RuleFinding        `json:"ruleFindings"`
	BlockingCount           int                  `json:"blockingCount"`
	WarningCount            int                  `json:"warningCount"`
	WarningDispositions     []WarningDisposition `json:"warningDispositions"`
	UnconfirmedWarningCount int                  `json:"unconfirmedWarningCount"`
	SampleRounds            []SampleRound        `json:"sampleRounds"`
	SamplePassedCount       int                  `json:"samplePassedCount"`
	SampleFailedCount       int                  `json:"sampleFailedCount"`
	SampleConditions        string               `json:"sampleConditions,omitempty"`
	SampleObservations      string               `json:"sampleObservations,omitempty"`
	SampleOutcome           SampleOutcome        `json:"sampleOutcome"`
	AssessedBy              string               `json:"assessedBy"`
	AssessedAt              time.Time            `json:"assessedAt"`
	SampledBy               string               `json:"sampledBy,omitempty"`
	SampledAt               *time.Time           `json:"sampledAt,omitempty"`
}

func (a CompatibilityAssessment) Clone() CompatibilityAssessment {
	a.RuleFindings = append([]RuleFinding(nil), a.RuleFindings...)
	a.WarningDispositions = append([]WarningDisposition(nil), a.WarningDispositions...)
	a.SampleRounds = append([]SampleRound(nil), a.SampleRounds...)
	a.SampledAt = cloneTimePtr(a.SampledAt)
	return a
}

func EvaluateCompatibility(id string, plan TreatmentPlanRevision, actor string, now time.Time) (CompatibilityAssessment, error) {
	a := CompatibilityAssessment{ID: id, PlanRevisionID: plan.ID, RuleFindings: []RuleFinding{}, WarningDispositions: []WarningDisposition{}, SampleRounds: []SampleRound{}, SampleOutcome: SamplePending, AssessedBy: strings.TrimSpace(actor), AssessedAt: now}
	if a.AssessedBy == "" {
		return a, invalid("actor", "核验员不能为空")
	}
	for _, material := range plan.Materials {
		if material.PH < 6 || material.PH > 8.5 {
			a.add(RuleFinding{Code: "PH_OUT_OF_RANGE", Level: FindingBlock, Material: material.Name, Message: fmt.Sprintf("材料 pH %.1f 超出古籍纸张安全范围", material.PH), Resolution: "改用 pH 6.0 至 8.5 的稳定材料"})
		}
		if !material.Reversible {
			a.add(RuleFinding{Code: "NOT_REVERSIBLE", Level: FindingBlock, Material: material.Name, Message: "材料或处理不可逆", Resolution: "选择经过验证的可逆材料或说明替代工艺"})
		}
		if material.ContainsMetal {
			a.add(RuleFinding{Code: "METAL_CATALYSIS", Level: FindingBlock, Material: material.Name, Message: "含金属成分可能催化纸张氧化", Resolution: "移除含金属材料"})
		}
		if material.WaterBased && containsAny(plan.PigmentConstraint, "水敏", "易晕染", "矿物颜料") {
			a.add(RuleFinding{Code: "WATER_SENSITIVE_PIGMENT", Level: FindingBlock, Material: material.Name, Message: "水性材料与颜料约束不相容", Resolution: "先固色或改用非水性方案"})
		}
		if material.PH >= 8 && containsAny(plan.PaperConstraint, "酸化", "脆化") {
			a.add(RuleFinding{Code: "ALKALINE_CAUTION", Level: FindingWarning, Material: material.Name, Message: "弱碱性材料用于酸化纸张时需控制局部浓度", Resolution: "在模拟试样记录浓度和色差"})
		}
	}
	if containsAny(plan.BindingConstraint, "不可拆", "原装帧") {
		for _, step := range plan.Steps {
			if containsAny(step.Description+step.Technique, "拆", "解体") {
				a.add(RuleFinding{Code: "BINDING_DISASSEMBLY", Level: FindingBlock, Message: "工艺步骤违反不可拆装帧约束", Resolution: "改用原位修复步骤"})
			}
		}
	}
	if len(a.RuleFindings) == 0 {
		a.add(RuleFinding{Code: "RULES_PASSED", Level: FindingInfo, Message: "材料与纸张、颜料和装帧约束未发现冲突"})
	}
	for index := range a.RuleFindings {
		a.RuleFindings[index].ID = fmt.Sprintf("%s_finding_%02d", id, index+1)
	}
	a.UnconfirmedWarningCount = a.WarningCount
	return a, nil
}

func (c *ConservationCase) ConfirmWarnings(inputs []WarningDisposition, actor string, now time.Time) error {
	if c.Status != StatusPendingSample {
		return ErrInvalidTransition
	}
	a, err := c.currentAssessment()
	if err != nil {
		return err
	}
	if len(inputs) == 0 {
		return invalid("warningDispositions", "至少提交一条警示处置")
	}
	warnings := map[string]RuleFinding{}
	for _, f := range a.RuleFindings {
		if f.Level == FindingWarning {
			warnings[f.ID] = f
		}
	}
	existing := map[string]int{}
	for index, d := range a.WarningDispositions {
		existing[d.FindingID] = index
	}
	seenInput := map[string]struct{}{}
	for i := range inputs {
		d := inputs[i]
		d.FindingID = strings.TrimSpace(d.FindingID)
		d.Method = strings.TrimSpace(d.Method)
		d.ControlMeasures = strings.TrimSpace(d.ControlMeasures)
		if _, ok := warnings[d.FindingID]; !ok {
			return invalid("warningDispositions", "第一个处置引用了不存在或非 WARNING 的规则发现："+d.FindingID)
		}
		if _, ok := seenInput[d.FindingID]; ok {
			return invalid("warningDispositions", "同一批次不得重复提交警示处置："+d.FindingID)
		}
		seenInput[d.FindingID] = struct{}{}
		if d.Method == "" {
			return invalid("warningDispositions.method", "警示处置方式不能为空")
		}
		if len([]rune(d.ControlMeasures)) < 8 {
			return invalid("warningDispositions.controlMeasures", "控制措施至少 8 个字符")
		}
		d.ConfirmedBy = strings.TrimSpace(actor)
		d.ConfirmedAt = now
		inputs[i] = d
		if index, ok := existing[d.FindingID]; ok {
			a.WarningDispositions[index] = d
		} else {
			a.WarningDispositions = append(a.WarningDispositions, d)
			existing[d.FindingID] = len(a.WarningDispositions) - 1
		}
	}
	a.UnconfirmedWarningCount = a.WarningCount - len(a.WarningDispositions)
	if a.UnconfirmedWarningCount < 0 {
		a.UnconfirmedWarningCount = 0
	}
	c.Advance(c.Status, now)
	return nil
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func (a *CompatibilityAssessment) add(f RuleFinding) {
	a.RuleFindings = append(a.RuleFindings, f)
	if f.Level == FindingBlock {
		a.BlockingCount++
	}
	if f.Level == FindingWarning {
		a.WarningCount++
	}
}

func (c *ConservationCase) AddAssessment(a CompatibilityAssessment, now time.Time) error {
	if c.Status != StatusPendingCompatibility {
		return ErrInvalidTransition
	}
	if a.PlanRevisionID != c.CurrentPlanRevisionID {
		return invalid("planRevisionID", "只能核验当前方案版本")
	}
	c.Assessments = append(c.Assessments, a)
	if a.BlockingCount > 0 {
		for _, f := range a.RuleFindings {
			if f.Level == FindingBlock {
				c.addRemediationItem(a.PlanRevisionID, "COMPATIBILITY_BLOCK", f.ID, "", f.Message)
			}
		}
		c.Advance(StatusReturned, now)
	} else {
		c.Advance(StatusPendingSample, now)
	}
	return nil
}

func (c *ConservationCase) RecordSample(conditions, observations string, outcome SampleOutcome, actor string, now time.Time) error {
	return c.AppendSampleRound(SampleRound{ID: "legacy-sample", MaterialBatch: "旧请求未单列批次", TemperatureC: 22, HumidityPercent: 50, DurationMinutes: 60, ColorDifference: "旧请求见观察", Deformation: "旧请求见观察", Observations: strings.TrimSpace(conditions) + "；" + strings.TrimSpace(observations), Outcome: outcome, PerformedBy: actor}, now)
}

func (c *ConservationCase) AppendSampleRound(round SampleRound, now time.Time) error {
	if c.Status != StatusPendingSample {
		return ErrInvalidTransition
	}
	round.MaterialBatch = strings.TrimSpace(round.MaterialBatch)
	round.ColorDifference = strings.TrimSpace(round.ColorDifference)
	round.Deformation = strings.TrimSpace(round.Deformation)
	round.Observations = strings.TrimSpace(round.Observations)
	round.PerformedBy = strings.TrimSpace(round.PerformedBy)
	if round.Outcome != SamplePass && round.Outcome != SampleFail {
		return invalid("sampleOutcome", "试样结论必须为 PASS 或 FAIL")
	}
	if round.PerformedBy == "" {
		return invalid("actor", "试样记录人不能为空")
	}
	a, err := c.currentAssessment()
	if err != nil {
		return err
	}
	if a.UnconfirmedWarningCount > 0 {
		missing := []string{}
		confirmed := map[string]struct{}{}
		for _, d := range a.WarningDispositions {
			confirmed[d.FindingID] = struct{}{}
		}
		for _, f := range a.RuleFindings {
			if f.Level == FindingWarning {
				if _, ok := confirmed[f.ID]; !ok {
					missing = append(missing, f.Code+"（缺少处置确认）")
				}
			}
		}
		return invalid("warningDispositions", "试样前仍有未确认警示："+strings.Join(missing, "、"))
	}
	plan, _ := c.CurrentPlan()
	expectedRound := len(a.SampleRounds) + 1
	if round.Round == 0 {
		round.Round = expectedRound
	}
	if round.Round != expectedRound {
		return invalid("round", fmt.Sprintf("试样轮次必须连续，下一轮应为 %d", expectedRound))
	}
	if round.ID == "" {
		return invalid("id", "试样轮次标识不能为空")
	}
	if round.MaterialBatch == "" {
		return invalid("materialBatch", "材料批次不能为空")
	}
	if round.TemperatureC < 5 || round.TemperatureC > 40 {
		return invalid("temperatureC", "温度应在 5 至 40℃")
	}
	if round.HumidityPercent < 20 || round.HumidityPercent > 80 {
		return invalid("humidityPercent", "相对湿度应在 20% 至 80%")
	}
	if round.DurationMinutes < 1 || round.DurationMinutes > 10080 {
		return invalid("durationMinutes", "作用时长应为 1 至 10080 分钟")
	}
	if len([]rune(round.Observations)) < 3 || round.ColorDifference == "" || round.Deformation == "" {
		return invalid("observations", "必须完整填写色差、形变和观察说明")
	}
	round.PerformedAt = now
	a.SampleRounds = append(a.SampleRounds, round)
	a.SampleConditions = fmt.Sprintf("%.1f℃ / %.1f%% / %d 分钟 / 批次 %s", round.TemperatureC, round.HumidityPercent, round.DurationMinutes, round.MaterialBatch)
	a.SampleObservations = round.Observations
	a.SampledBy = round.PerformedBy
	a.SampledAt = &now
	if round.Outcome == SampleFail {
		a.SampleFailedCount++
		a.SampleOutcome = SampleFail
		c.addRemediationItem(a.PlanRevisionID, "SAMPLE_FAILURE", round.ID, "", fmt.Sprintf("第 %d 轮试样失败：%s", round.Round, round.Observations))
		c.Advance(StatusReturned, now)
	} else {
		a.SamplePassedCount++
		if a.SamplePassedCount >= plan.RequiredSampleRounds {
			a.SampleOutcome = SamplePass
			c.Advance(StatusPendingReview, now)
		} else {
			a.SampleOutcome = SamplePending
			c.Advance(StatusPendingSample, now)
		}
	}
	return nil
}

func remediationID(planID, source, sourceID string) string {
	sum := sha256.Sum256([]byte(planID + "\x1f" + source + "\x1f" + sourceID))
	return "rem_" + hex.EncodeToString(sum[:8])
}
func (c *ConservationCase) addRemediationItem(planID, source, sourceID, folio, description string) {
	id := remediationID(planID, source, sourceID)
	for _, item := range c.RemediationItems {
		if item.ID == id {
			return
		}
	}
	c.RemediationItems = append(c.RemediationItems, RemediationItem{ID: id, PlanRevisionID: planID, Source: source, SourceID: sourceID, FolioRef: folio, Description: description, Required: true})
}

func (c *ConservationCase) currentAssessment() (*CompatibilityAssessment, error) {
	for i := len(c.Assessments) - 1; i >= 0; i-- {
		if c.Assessments[i].PlanRevisionID == c.CurrentPlanRevisionID {
			return &c.Assessments[i], nil
		}
	}
	return nil, invalid("assessment", "当前方案没有相容性核验记录")
}

func (c *ConservationCase) CurrentAssessment() (*CompatibilityAssessment, error) {
	return c.currentAssessment()
}
