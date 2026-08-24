package store

import (
	"encoding/json"
	"time"

	"manuscript-conservation-gate/internal/domain"
)

const schemaVersion = 1

type transactionEnvelope struct {
	SchemaVersion int                      `json:"schemaVersion"`
	Sequence      int64                    `json:"sequence"`
	PreviousHash  string                   `json:"previousHash"`
	Hash          string                   `json:"hash"`
	CommittedAt   time.Time                `json:"committedAt"`
	Event         domain.Event             `json:"event"`
	Case          *domain.ConservationCase `json:"case"`
	Idempotency   domain.IdempotencyRecord `json:"idempotency"`
}

type projectionFile struct {
	SchemaVersion int                                 `json:"schemaVersion"`
	Sequence      int64                               `json:"sequence"`
	LogHeadHash   string                              `json:"logHeadHash"`
	Cases         map[string]*domain.ConservationCase `json:"cases"`
	Idempotency   map[string]domain.IdempotencyRecord `json:"idempotency"`
}

type hashableEnvelope struct {
	SchemaVersion int                      `json:"schemaVersion"`
	Sequence      int64                    `json:"sequence"`
	PreviousHash  string                   `json:"previousHash"`
	CommittedAt   time.Time                `json:"committedAt"`
	Event         domain.Event             `json:"event"`
	Case          *domain.ConservationCase `json:"case"`
	Idempotency   domain.IdempotencyRecord `json:"idempotency"`
}

func envelopeHash(e transactionEnvelope) (string, error) {
	return domain.HashJSON(hashableEnvelope{e.SchemaVersion, e.Sequence, e.PreviousHash, e.CommittedAt, e.Event, e.Case, e.Idempotency})
}

func cloneRaw(value json.RawMessage) json.RawMessage { return append(json.RawMessage(nil), value...) }
