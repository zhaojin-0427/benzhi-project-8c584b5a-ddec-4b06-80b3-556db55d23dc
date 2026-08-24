package store

import (
	"errors"
	"fmt"
	"os"

	"manuscript-conservation-gate/internal/domain"
)

func (s *Store) recover() error {
	transactions, head, err := replayLog(s.logPath)
	if err != nil {
		return fmt.Errorf("恢复事件日志: %w", err)
	}
	if projection, snapshotErr := readSnapshot(s.snapshotPath); snapshotErr == nil && projection.Sequence == int64(len(transactions)) && projection.LogHeadHash == head {
		s.cases, s.idempotency = projection.Cases, projection.Idempotency
		s.sequence, s.headHash = projection.Sequence, projection.LogHeadHash
		return s.validateProjection(transactions)
	} else if snapshotErr != nil && !errors.Is(snapshotErr, os.ErrNotExist) {
		// 损坏或过时的投影不是事实来源，下面依据已校验事件重建。
	}
	s.cases = map[string]*domain.ConservationCase{}
	s.idempotency = map[string]domain.IdempotencyRecord{}
	for _, transaction := range transactions {
		s.cases[transaction.Case.ID] = transaction.Case.Clone()
		s.idempotency[idempotencyID(transaction.Idempotency.Scope, transaction.Idempotency.Key)] = transaction.Idempotency
	}
	s.sequence, s.headHash = int64(len(transactions)), head
	if len(transactions) > 0 {
		if err := s.writeSnapshot(); err != nil {
			return fmt.Errorf("重建投影: %w", err)
		}
	}
	return nil
}

func (s *Store) validateProjection(transactions []transactionEnvelope) error {
	latest := map[string]*domain.ConservationCase{}
	for _, transaction := range transactions {
		latest[transaction.Case.ID] = transaction.Case
	}
	if len(latest) != len(s.cases) {
		return s.rebuildAfterProjectionMismatch(transactions)
	}
	for id, expected := range latest {
		actual, ok := s.cases[id]
		if !ok || actual.Version != expected.Version || actual.AuditHeadHash != expected.AuditHeadHash {
			return s.rebuildAfterProjectionMismatch(transactions)
		}
	}
	return nil
}

func (s *Store) rebuildAfterProjectionMismatch(transactions []transactionEnvelope) error {
	s.cases = map[string]*domain.ConservationCase{}
	s.idempotency = map[string]domain.IdempotencyRecord{}
	for _, transaction := range transactions {
		s.cases[transaction.Case.ID] = transaction.Case.Clone()
		s.idempotency[idempotencyID(transaction.Idempotency.Scope, transaction.Idempotency.Key)] = transaction.Idempotency
	}
	return s.writeSnapshot()
}
