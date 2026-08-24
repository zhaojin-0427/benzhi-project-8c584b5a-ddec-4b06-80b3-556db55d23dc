package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"manuscript-conservation-gate/internal/application"
	"manuscript-conservation-gate/internal/domain"
)

type selfcheckClient struct {
	baseURL string
	client  *http.Client
	caseID  string
	version int64
	step    int
}

type caseDetail struct {
	Case            *domain.ConservationCase `json:"case"`
	ReviewEvidence  domain.EvidenceSnapshot  `json:"reviewEvidence"`
	ReleaseEvidence domain.EvidenceSnapshot  `json:"releaseEvidence"`
}

func runSelfcheck(parent context.Context, baseURL string) error {
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	c := &selfcheckClient{baseURL: baseURL, client: &http.Client{Timeout: 4 * time.Second}}
	if err := c.get(ctx, "/healthz", nil); err != nil {
		return fmt.Errorf("健康检查: %w", err)
	}
	created, err := c.post(ctx, "/api/cases", map[string]any{
		"expectedVersion": 0, "idempotencyKey": c.key(), "actor": "自检修复师", "role": domain.RoleConservator,
		"accessionCode": "SELF-CHECK-001", "shelfLocation": "自检库 A-01", "title": "自检古籍", "responsibleConservator": "自检修复师",
	})
	if err != nil {
		return fmt.Errorf("建档: %w", err)
	}
	c.update(created)
	damaged, err := c.action(ctx, "damages", map[string]any{"actor": "自检修复师", "role": domain.RoleConservator,
		"records": []domain.DamageInput{{FolioRef: "卷一第 1 叶", DamageType: "撕裂", Extent: "书口约 2 cm", Severity: domain.SeverityHigh, EvidenceNote: "斜光照片显示纤维断裂"}, {FolioRef: "卷一第 2 叶", DamageType: "虫蛀", Extent: "版心约 1 cm", Severity: domain.SeverityModerate, EvidenceNote: "透射照片显示纸张缺失"}}})
	if err != nil {
		return fmt.Errorf("登记损伤: %w", err)
	}
	if _, err := c.action(ctx, "plans", map[string]any{"actor": "自检修复师", "role": domain.RoleConservator,
		"steps":           []domain.TreatmentStep{{Order: 1, Description: "清洁后原位托补", Technique: "可逆托补", DamageIDs: []string{damaged.Case.Damages[0].ID}}},
		"materials":       []domain.Material{{Name: "楮皮补纸", Category: "补纸", PH: 7.2, Reversible: true}},
		"paperConstraint": "轻度酸化", "pigmentConstraint": "无水敏颜料", "bindingConstraint": "保持原装帧", "requiredSampleRounds": 2, "changeReason": ""}); err != nil {
		return fmt.Errorf("编制方案: %w", err)
	}
	if _, err := c.action(ctx, "submit", map[string]any{"actor": "自检修复师", "role": domain.RoleConservator}); err != nil {
		return fmt.Errorf("提交方案: %w", err)
	}
	if _, err := c.action(ctx, "assessments", map[string]any{"actor": "自检核验员", "role": domain.RoleVerifier}); err != nil {
		return fmt.Errorf("材料核验: %w", err)
	}
	for round := 1; round <= 2; round++ {
		if _, err := c.action(ctx, "samples", map[string]any{"actor": "自检修复师", "role": domain.RoleConservator,
			"round": round, "materialBatch": "SELF-BATCH-01", "temperatureC": 22, "humidityPercent": 50, "durationMinutes": 60, "colorDifference": "无明显色差", "deformation": "无明显形变", "observations": "干燥后无色差且粘结稳定", "sampleOutcome": domain.SamplePass}); err != nil {
			return fmt.Errorf("第 %d 轮试样评估: %w", round, err)
		}
	}
	var detail caseDetail
	if err := c.get(ctx, "/api/cases/"+c.caseID, &detail); err != nil {
		return fmt.Errorf("读取复核证据: %w", err)
	}
	if _, err := c.action(ctx, "reviews", map[string]any{"actor": "自检专家", "role": domain.RoleExpert,
		"decision": domain.ReviewApproved, "comments": []domain.ReviewComment{}, "planRevisionID": detail.ReviewEvidence.PlanRevisionID, "planContentHash": detail.ReviewEvidence.PlanContentHash, "evidenceDigest": detail.ReviewEvidence.Digest, "confirmedEvidenceItemIDs": detail.ReviewEvidence.ItemIDs}); err != nil {
		return fmt.Errorf("专家复核: %w", err)
	}
	detail = caseDetail{}
	if err := c.get(ctx, "/api/cases/"+c.caseID, &detail); err != nil {
		return fmt.Errorf("读取放行证据: %w", err)
	}
	released, err := c.action(ctx, "release", map[string]any{"actor": "自检专家", "role": domain.RoleExpert, "planRevisionID": detail.ReleaseEvidence.PlanRevisionID, "planContentHash": detail.ReleaseEvidence.PlanContentHash, "evidenceDigest": detail.ReleaseEvidence.Digest})
	if err != nil {
		return fmt.Errorf("修复放行: %w", err)
	}
	if released.Credential == nil || released.Case.Status != domain.StatusReleased {
		return fmt.Errorf("放行未返回不可变凭据")
	}
	var verified application.VerificationResult
	if err := c.get(ctx, "/api/cases/"+c.caseID+"/credential/verify", &verified); err != nil {
		return fmt.Errorf("验证凭据: %w", err)
	}
	if !verified.Valid {
		return fmt.Errorf("凭据验证返回无效: %s", verified.Message)
	}
	if !verified.Checks.PlanDigest || !verified.Checks.Frozen || !verified.Checks.EvidenceDigest || !verified.Checks.AuditHead || !verified.Checks.Signature {
		return fmt.Errorf("凭据分项验真未全部通过: %+v", verified.Checks)
	}
	return nil
}

func (c *selfcheckClient) key() string { c.step++; return fmt.Sprintf("selfcheck-%02d-fixed", c.step) }

func (c *selfcheckClient) action(ctx context.Context, suffix string, fields map[string]any) (application.OperationResult, error) {
	fields["expectedVersion"] = c.version
	fields["idempotencyKey"] = c.key()
	result, err := c.post(ctx, "/api/cases/"+c.caseID+"/"+suffix, fields)
	if err == nil {
		c.update(result)
	}
	return result, err
}

func (c *selfcheckClient) update(result application.OperationResult) {
	if result.Case != nil {
		c.caseID, c.version = result.Case.ID, result.Case.Version
	}
}

func (c *selfcheckClient) post(ctx context.Context, path string, payload any) (application.OperationResult, error) {
	var result application.OperationResult
	data, err := json.Marshal(payload)
	if err != nil {
		return result, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return result, err
	}
	request.Header.Set("Content-Type", "application/json")
	if err := c.do(request, &result); err != nil {
		return result, err
	}
	return result, nil
}

func (c *selfcheckClient) get(ctx context.Context, path string, destination any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	return c.do(request, destination)
}

func (c *selfcheckClient) do(request *http.Request, destination any) error {
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", response.StatusCode, string(body))
	}
	if destination == nil {
		return nil
	}
	if err := json.Unmarshal(body, destination); err != nil {
		return fmt.Errorf("解析响应: %w", err)
	}
	return nil
}
