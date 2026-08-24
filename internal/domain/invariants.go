package domain

import (
	"fmt"
	"strings"
)

// ValidateAggregate 检查持久化边界上的完整业务投影。它与命令时的局部
// 校验互补，用于阻止哈希链内部出现结构自洽但业务关系损坏的数据。
func ValidateAggregate(c *ConservationCase) error {
	if c == nil {
		return integrity("档案投影为空")
	}
	if strings.TrimSpace(c.ID) == "" || strings.TrimSpace(c.AccessionCode) == "" {
		return integrity("档案缺少标识或馆藏编号")
	}
	if c.Version < 1 {
		return integrity("档案版本必须为正整数")
	}
	if c.CreatedAt.IsZero() || c.UpdatedAt.IsZero() || c.UpdatedAt.Before(c.CreatedAt) {
		return integrity("档案时间范围无效")
	}
	if !validStatus(c.Status) {
		return integrity("档案状态无效")
	}
	if err := uniqueDamageIDs(c); err != nil {
		return err
	}
	if err := validatePlans(c); err != nil {
		return err
	}
	if err := validateAssessments(c); err != nil {
		return err
	}
	if err := validateReviews(c); err != nil {
		return err
	}
	if err := validateRemediation(c); err != nil {
		return err
	}
	if err := validateAuditProjection(c); err != nil {
		return err
	}
	if err := validateStateEvidence(c); err != nil {
		return err
	}
	return nil
}

func validStatus(status Status) bool {
	switch status {
	case StatusDraft, StatusPendingCompatibility, StatusPendingSample, StatusPendingReview, StatusReturned, StatusApproved, StatusReleased:
		return true
	default:
		return false
	}
}

func ValidStatus(status Status) bool { return validStatus(status) }

func uniqueDamageIDs(c *ConservationCase) error {
	ids := map[string]struct{}{}
	for _, damage := range c.Damages {
		if damage.ID == "" || damage.CaseID != c.ID {
			return integrity("损伤记录标识或档案引用无效")
		}
		if _, exists := ids[damage.ID]; exists {
			return integrity("损伤记录标识重复")
		}
		ids[damage.ID] = struct{}{}
		if damage.FolioRef == "" || damage.DamageType == "" || damage.Extent == "" || damage.EvidenceNote == "" {
			return integrity("损伤记录缺少业务字段")
		}
		if damage.Severity != SeverityLow && damage.Severity != SeverityModerate && damage.Severity != SeverityHigh {
			return integrity("损伤严重度无效")
		}
		if damage.RecordedAt.IsZero() {
			return integrity("损伤记录时间无效")
		}
	}
	expected := NewDamageSummary(c.Damages)
	if c.DamageSummary.Total != expected.Total || !sameStringCounts(c.DamageSummary.ByFolio, expected.ByFolio) || !sameStringCounts(c.DamageSummary.ByType, expected.ByType) || !sameSeverityCounts(c.DamageSummary.BySeverity, expected.BySeverity) {
		return integrity("损伤基线汇总与明细不一致")
	}
	return nil
}

func sameStringCounts(left, right map[string]int) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}
func sameSeverityCounts(left, right map[Severity]int) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

func validatePlans(c *ConservationCase) error {
	ids := map[string]struct{}{}
	previousID := ""
	for index, plan := range c.PlanRevisions {
		if plan.ID == "" || plan.CaseID != c.ID {
			return integrity("方案版本标识或档案引用无效")
		}
		if _, exists := ids[plan.ID]; exists {
			return integrity("方案版本标识重复")
		}
		ids[plan.ID] = struct{}{}
		if plan.RevisionNumber != index+1 {
			return integrity("方案修订号不连续")
		}
		if index == 0 && plan.SupersedesRevisionID != "" {
			return integrity("首版方案不得引用被替代版本")
		}
		if index > 0 && plan.SupersedesRevisionID != previousID {
			return integrity("方案版本继承关系不连续")
		}
		if len(plan.Steps) == 0 || len(plan.Materials) == 0 {
			return integrity("方案缺少步骤或材料")
		}
		for stepIndex, step := range plan.Steps {
			if step.Order != stepIndex+1 || step.Description == "" || step.Technique == "" {
				return integrity("方案步骤顺序或内容无效")
			}
			seenDamage := map[string]struct{}{}
			for _, id := range step.DamageIDs {
				if _, ok := seenDamage[id]; ok {
					return integrity("方案步骤重复引用损伤")
				}
				seenDamage[id] = struct{}{}
			}
		}
		coverage, err := calculateCoverage(c, plan.Steps)
		if err != nil {
			return integrity("方案步骤引用未知损伤")
		}
		if !sameStrings(coverage.CoveredDamageIDs, plan.Coverage.CoveredDamageIDs) || !sameStrings(coverage.UncoveredDamageIDs, plan.Coverage.UncoveredDamageIDs) || !sameStrings(coverage.UncoveredHighDamageIDs, plan.Coverage.UncoveredHighDamageIDs) {
			return integrity("方案损伤覆盖投影不一致")
		}
		for _, material := range plan.Materials {
			if material.Name == "" || material.Category == "" || material.PH < 0 || material.PH > 14 {
				return integrity("方案材料内容无效")
			}
		}
		if plan.ContentHash != PlanDigest(plan) {
			return integrity("方案规范化摘要不一致")
		}
		if plan.SemanticHash != "" && plan.SemanticHash != PlanSemanticDigest(plan) {
			return integrity("方案语义摘要不一致")
		}
		if plan.Frozen && (c.Status != StatusReleased || plan.ID != c.CurrentPlanRevisionID) {
			return integrity("仅已放行的当前方案可以冻结")
		}
		previousID = plan.ID
	}
	if len(c.PlanRevisions) == 0 {
		if c.CurrentPlanRevisionID != "" {
			return integrity("无方案档案存在当前方案引用")
		}
	} else if c.CurrentPlanRevisionID != c.PlanRevisions[len(c.PlanRevisions)-1].ID {
		return integrity("当前方案必须指向最新修订版")
	}
	return nil
}

func validateAssessments(c *ConservationCase) error {
	planIDs := map[string]struct{}{}
	for _, plan := range c.PlanRevisions {
		planIDs[plan.ID] = struct{}{}
	}
	assessmentIDs := map[string]struct{}{}
	perPlan := map[string]int{}
	for _, assessment := range c.Assessments {
		if assessment.ID == "" {
			return integrity("核验记录缺少标识")
		}
		if _, exists := assessmentIDs[assessment.ID]; exists {
			return integrity("核验记录标识重复")
		}
		assessmentIDs[assessment.ID] = struct{}{}
		if _, exists := planIDs[assessment.PlanRevisionID]; !exists {
			return integrity("核验记录引用未知方案")
		}
		perPlan[assessment.PlanRevisionID]++
		if perPlan[assessment.PlanRevisionID] > 1 {
			return integrity("同一方案版本存在重复核验")
		}
		blocks, warnings := 0, 0
		findingIDs := map[string]FindingLevel{}
		for _, finding := range assessment.RuleFindings {
			if finding.Code == "" || finding.Message == "" {
				return integrity("规则发现缺少编码或说明")
			}
			if finding.Level == FindingBlock {
				blocks++
			} else if finding.Level == FindingWarning {
				warnings++
			} else if finding.Level != FindingInfo {
				return integrity("规则发现级别无效")
			}
			if finding.ID != "" {
				if _, ok := findingIDs[finding.ID]; ok {
					return integrity("规则发现标识重复")
				}
				findingIDs[finding.ID] = finding.Level
			}
		}
		if blocks != assessment.BlockingCount || warnings != assessment.WarningCount {
			return integrity("规则发现统计不一致")
		}
		disposed := map[string]struct{}{}
		for _, d := range assessment.WarningDispositions {
			if findingIDs[d.FindingID] != FindingWarning {
				return integrity("警示处置引用无效规则发现")
			}
			if _, ok := disposed[d.FindingID]; ok {
				return integrity("警示处置重复")
			}
			disposed[d.FindingID] = struct{}{}
		}
		if assessment.UnconfirmedWarningCount != warnings-len(disposed) {
			return integrity("未确认警示统计不一致")
		}
		for i, round := range assessment.SampleRounds {
			if round.Round != i+1 || round.ID == "" || round.PerformedAt.IsZero() {
				return integrity("试样轮次顺序或标识无效")
			}
		}
		if assessment.SampleOutcome != SamplePending && assessment.SampleOutcome != SamplePass && assessment.SampleOutcome != SampleFail {
			return integrity("试样结论无效")
		}
		passed, failed := 0, 0
		for _, round := range assessment.SampleRounds {
			if round.Outcome == SamplePass {
				passed++
			} else if round.Outcome == SampleFail {
				failed++
			}
		}
		if passed != assessment.SamplePassedCount || failed != assessment.SampleFailedCount {
			return integrity("试样轮次汇总不一致")
		}
		if assessment.SampleOutcome != SamplePending && (assessment.SampledAt == nil || assessment.SampleConditions == "" || assessment.SampleObservations == "") {
			return integrity("已完成试样缺少条件、观察或时间")
		}
	}
	return nil
}

func validateReviews(c *ConservationCase) error {
	planIDs := map[string]struct{}{}
	for _, plan := range c.PlanRevisions {
		planIDs[plan.ID] = struct{}{}
	}
	reviewIDs := map[string]struct{}{}
	perPlan := map[string]int{}
	for _, review := range c.Reviews {
		if review.ID == "" {
			return integrity("复核记录缺少标识")
		}
		if _, exists := reviewIDs[review.ID]; exists {
			return integrity("复核记录标识重复")
		}
		reviewIDs[review.ID] = struct{}{}
		if _, exists := planIDs[review.PlanRevisionID]; !exists {
			return integrity("复核记录引用未知方案")
		}
		perPlan[review.PlanRevisionID]++
		if perPlan[review.PlanRevisionID] > 1 {
			return integrity("同一方案版本存在重复复核")
		}
		if review.Decision != ReviewApproved && review.Decision != ReviewReturned {
			return integrity("复核决定无效")
		}
		if review.Decision == ReviewReturned && len(review.Comments) == 0 {
			return integrity("退回复核缺少结构化意见")
		}
		if review.EvidenceSnapshot.Digest == "" || review.EvidenceSnapshot.Digest != review.EvidenceSnapshot.ComputeDigest() {
			return integrity("复核证据快照摘要不一致")
		}
	}
	return nil
}

func validateRemediation(c *ConservationCase) error {
	plans := map[string]struct{}{}
	for _, p := range c.PlanRevisions {
		plans[p.ID] = struct{}{}
	}
	ids := map[string]struct{}{}
	for _, item := range c.RemediationItems {
		if item.ID == "" || item.Source == "" || item.SourceID == "" || item.Description == "" {
			return integrity("整改项缺少业务字段")
		}
		if _, ok := ids[item.ID]; ok {
			return integrity("整改项标识重复")
		}
		ids[item.ID] = struct{}{}
		if _, ok := plans[item.PlanRevisionID]; !ok {
			return integrity("整改项引用未知方案")
		}
		if item.Closed {
			if _, ok := plans[item.ClosedByPlanRevisionID]; !ok || item.ClosedAt == nil || item.ClosedBy == "" {
				return integrity("已关闭整改项缺少处置版本、人员或时间")
			}
		} else if item.ClosedByPlanRevisionID != "" || item.ClosedAt != nil {
			return integrity("未关闭整改项包含关闭信息")
		}
	}
	return nil
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func validateAuditProjection(c *ConservationCase) error {
	if len(c.AuditTrail) == 0 {
		return integrity("档案缺少审计记录")
	}
	last := c.AuditTrail[len(c.AuditTrail)-1]
	if last.Hash == "" || last.Hash != c.AuditHeadHash {
		return integrity("档案审计链头与末条记录不一致")
	}
	if last.EntityVersion != c.Version {
		return integrity("末条审计记录未覆盖当前档案版本")
	}
	for _, record := range c.AuditTrail {
		if record.CaseID != c.ID {
			return integrity("审计记录引用其他档案")
		}
		if !ValidRole(record.Role) || record.Actor == "" || record.EventType == "" {
			return integrity("审计记录操作者、角色或事件类型无效")
		}
	}
	return nil
}

func validateStateEvidence(c *ConservationCase) error {
	if c.Status != StatusDraft && c.Status != StatusReturned && len(c.PlanRevisions) == 0 {
		return integrity("当前状态要求存在方案")
	}
	if c.Status == StatusPendingCompatibility {
		plan, err := c.CurrentPlan()
		if err != nil || plan.SubmittedAt == nil {
			return integrity("待核验状态缺少已提交方案")
		}
	}
	assessment, assessmentErr := c.currentAssessment()
	if c.Status == StatusPendingSample && (assessmentErr != nil || assessment.BlockingCount != 0 || assessment.SampleOutcome != SamplePending) {
		return integrity("待试样状态与核验证据不一致")
	}
	if c.Status == StatusPendingReview && (assessmentErr != nil || assessment.BlockingCount != 0 || assessment.SampleOutcome != SamplePass) {
		return integrity("待复核状态与核验、试样证据不一致")
	}
	if c.Status == StatusApproved && !c.HasApprovedCurrentPlan() {
		return integrity("已批准状态缺少当前方案批准记录")
	}
	if c.Status == StatusReleased {
		if c.Credential == nil {
			return integrity("已放行状态缺少凭据")
		}
		if !c.HasApprovedCurrentPlan() {
			return integrity("已放行状态缺少专家批准")
		}
		plan, err := c.CurrentPlan()
		if err != nil || !plan.Frozen {
			return integrity("已放行状态缺少冻结方案")
		}
		if c.Credential.CaseID != c.ID || c.Credential.FrozenPlanRevisionID != c.CurrentPlanRevisionID || c.Credential.ContentDigest != plan.ContentHash || c.Credential.EvidenceDigest == "" || c.Credential.AuditHeadHash != c.AuditHeadHash {
			return integrity("放行凭据与档案证据不一致")
		}
		evidence, err := c.GateEvidenceSnapshot()
		if err != nil || evidence.Digest != c.Credential.EvidenceDigest || !sameStrings(evidence.ItemIDs, c.Credential.EvidenceItemIDs) {
			return integrity("放行凭据的证据清单摘要不一致")
		}
	} else if c.Credential != nil {
		return integrity("未放行档案不得持有放行凭据")
	}
	return nil
}

func integrity(message string) error { return fmt.Errorf("%w: %s", ErrIntegrity, message) }
