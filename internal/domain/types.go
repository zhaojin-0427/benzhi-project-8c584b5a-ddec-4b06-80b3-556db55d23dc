package domain

import (
	"encoding/json"
	"time"
)

type Status string

const (
	StatusDraft                Status = "DRAFT"
	StatusPendingCompatibility Status = "PENDING_COMPATIBILITY"
	StatusPendingSample        Status = "PENDING_SAMPLE"
	StatusPendingReview        Status = "PENDING_REVIEW"
	StatusReturned             Status = "RETURNED_REMEDIATION"
	StatusApproved             Status = "APPROVED"
	StatusReleased             Status = "RELEASED"
)

type Role string

const (
	RoleConservator Role = "conservator"
	RoleVerifier    Role = "verifier"
	RoleExpert      Role = "expert"
)

type Severity string

const (
	SeverityLow      Severity = "LOW"
	SeverityModerate Severity = "MODERATE"
	SeverityHigh     Severity = "HIGH"
)

type SampleOutcome string

const (
	SamplePending SampleOutcome = "PENDING"
	SamplePass    SampleOutcome = "PASS"
	SampleFail    SampleOutcome = "FAIL"
)

type ReviewDecision string

const (
	ReviewApproved ReviewDecision = "APPROVE"
	ReviewReturned ReviewDecision = "RETURN"
)

type Event struct {
	ID         string          `json:"id"`
	CaseID     string          `json:"caseID"`
	Type       string          `json:"type"`
	Actor      string          `json:"actor"`
	Role       Role            `json:"role"`
	Reason     string          `json:"reason"`
	Version    int64           `json:"version"`
	OccurredAt time.Time       `json:"occurredAt"`
	Data       json.RawMessage `json:"data,omitempty"`
}

type AuditRecord struct {
	Sequence      int64     `json:"sequence"`
	CaseID        string    `json:"caseID"`
	EventType     string    `json:"eventType"`
	Actor         string    `json:"actor"`
	Role          Role      `json:"role"`
	Reason        string    `json:"reason"`
	EntityVersion int64     `json:"entityVersion"`
	OccurredAt    time.Time `json:"occurredAt"`
	PreviousHash  string    `json:"previousHash"`
	Hash          string    `json:"hash"`
}

type IdempotencyRecord struct {
	Scope       string          `json:"scope"`
	Key         string          `json:"key"`
	PayloadHash string          `json:"payloadHash"`
	Response    json.RawMessage `json:"response"`
	CreatedAt   time.Time       `json:"createdAt"`
}

type CommitRequest struct {
	Scope           string            `json:"scope"`
	IdempotencyKey  string            `json:"idempotencyKey"`
	PayloadHash     string            `json:"payloadHash"`
	ExpectedVersion int64             `json:"expectedVersion"`
	Case            *ConservationCase `json:"case"`
	Event           Event             `json:"event"`
	Response        json.RawMessage   `json:"response"`
}

type CommitResult struct {
	Response  json.RawMessage
	Duplicate bool
}
