package application

import "manuscript-conservation-gate/internal/domain"

type CommandMeta struct {
	ExpectedVersion int64       `json:"expectedVersion"`
	IdempotencyKey  string      `json:"idempotencyKey"`
	Actor           string      `json:"actor"`
	Role            domain.Role `json:"role"`
	Reason          string      `json:"reason"`
}

type CreateCaseCommand struct {
	CommandMeta
	AccessionCode          string `json:"accessionCode"`
	ShelfLocation          string `json:"shelfLocation"`
	Title                  string `json:"title"`
	ResponsibleConservator string `json:"responsibleConservator"`
}

type AddDamageCommand struct {
	CommandMeta
	Records      []domain.DamageInput `json:"records,omitempty"`
	FolioRef     string               `json:"folioRef"`
	DamageType   string               `json:"damageType"`
	Extent       string               `json:"extent"`
	Severity     domain.Severity      `json:"severity"`
	EvidenceNote string               `json:"evidenceNote"`
}

type CreatePlanCommand struct {
	CommandMeta
	Steps                  []domain.TreatmentStep         `json:"steps"`
	Materials              []domain.Material              `json:"materials"`
	PaperConstraint        string                         `json:"paperConstraint"`
	PigmentConstraint      string                         `json:"pigmentConstraint"`
	BindingConstraint      string                         `json:"bindingConstraint"`
	ChangeReason           string                         `json:"changeReason"`
	RequiredSampleRounds   int                            `json:"requiredSampleRounds"`
	RemediationResolutions []domain.RemediationResolution `json:"remediationResolutions,omitempty"`
}

type SubmitPlanCommand struct{ CommandMeta }
type AssessCommand struct {
	CommandMeta
	WarningDispositions []domain.WarningDisposition `json:"warningDispositions,omitempty"`
}

type RecordSampleCommand struct {
	CommandMeta
	Round              int                  `json:"round,omitempty"`
	MaterialBatch      string               `json:"materialBatch,omitempty"`
	TemperatureC       float64              `json:"temperatureC,omitempty"`
	HumidityPercent    float64              `json:"humidityPercent,omitempty"`
	DurationMinutes    int                  `json:"durationMinutes,omitempty"`
	ColorDifference    string               `json:"colorDifference,omitempty"`
	Deformation        string               `json:"deformation,omitempty"`
	Observations       string               `json:"observations,omitempty"`
	SampleConditions   string               `json:"sampleConditions"`
	SampleObservations string               `json:"sampleObservations"`
	SampleOutcome      domain.SampleOutcome `json:"sampleOutcome"`
}

type ReviewCommand struct {
	CommandMeta
	Decision                 domain.ReviewDecision  `json:"decision"`
	Comments                 []domain.ReviewComment `json:"comments"`
	PlanRevisionID           string                 `json:"planRevisionID,omitempty"`
	PlanContentHash          string                 `json:"planContentHash,omitempty"`
	EvidenceDigest           string                 `json:"evidenceDigest,omitempty"`
	ConfirmedEvidenceItemIDs []string               `json:"confirmedEvidenceItemIDs,omitempty"`
}

type ReleaseCommand struct {
	CommandMeta
	PlanRevisionID  string `json:"planRevisionID"`
	PlanContentHash string `json:"planContentHash"`
	EvidenceDigest  string `json:"evidenceDigest"`
}

type OperationResult struct {
	Case       *domain.ConservationCase  `json:"case"`
	Credential *domain.ReleaseCredential `json:"credential,omitempty"`
	Replayed   bool                      `json:"replayed,omitempty"`
}

type VerificationResult struct {
	Valid      bool                      `json:"valid"`
	Credential *domain.ReleaseCredential `json:"credential,omitempty"`
	Message    string                    `json:"message"`
	Checks     VerificationChecks        `json:"checks"`
}

type VerificationChecks struct {
	PlanDigest     bool `json:"planDigest"`
	Frozen         bool `json:"frozen"`
	EvidenceDigest bool `json:"evidenceDigest"`
	AuditHead      bool `json:"auditHead"`
	Signature      bool `json:"signature"`
}

type CaseQuery struct {
	Keyword                string
	ResponsibleConservator string
	Status                 domain.Status
	Page                   int
	PageSize               int
}
type CasePage struct {
	Cases             []*domain.ConservationCase `json:"cases"`
	Total             int                        `json:"total"`
	Page              int                        `json:"page"`
	PageSize          int                        `json:"pageSize"`
	Counts            map[domain.Status]int      `json:"counts"`
	ProjectionVersion string                     `json:"projectionVersion"`
}
