package store

import (
	"errors"
	"fmt"
	"os"
	"reflect"

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
	return s.rebuildFromLog(transactions, head)
}

// rebuildFromLog 依据已校验事件日志重建内存投影，并在存在事务时持久化新快照。
func (s *Store) rebuildFromLog(transactions []transactionEnvelope, head string) error {
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

// validateProjection 将快照中的档案与幂等记录与已校验事件日志推导出的结果
// 逐一核对。任何业务字段、嵌套集合或幂等记录的偏差都会触发从日志重建，
// 从而保证 sequence、logHeadHash、档案 Version 和 AuditHeadHash 之外的字段
// 被篡改时仍能恢复到事实来源。
func (s *Store) validateProjection(transactions []transactionEnvelope) error {
	expectedCases, expectedIdempotency := projectFromTransactions(transactions)
	if len(expectedCases) != len(s.cases) || len(expectedIdempotency) != len(s.idempotency) {
		return s.rebuildAfterProjectionMismatch(transactions)
	}
	for id, expected := range expectedCases {
		actual, ok := s.cases[id]
		if !ok || !reflect.DeepEqual(expected, actual) {
			return s.rebuildAfterProjectionMismatch(transactions)
		}
	}
	for id, expected := range expectedIdempotency {
		actual, ok := s.idempotency[id]
		if !ok || !reflect.DeepEqual(expected, actual) {
			return s.rebuildAfterProjectionMismatch(transactions)
		}
	}
	return nil
}

// projectFromTransactions 依据事件事务序列推导出当前档案与幂等记录投影。
// 返回的档案为日志中事务所持有实例的直接引用，仅供只读比对使用。
func projectFromTransactions(transactions []transactionEnvelope) (map[string]*domain.ConservationCase, map[string]domain.IdempotencyRecord) {
	cases := map[string]*domain.ConservationCase{}
	idempotency := map[string]domain.IdempotencyRecord{}
	for _, transaction := range transactions {
		cases[transaction.Case.ID] = transaction.Case
		idempotency[idempotencyID(transaction.Idempotency.Scope, transaction.Idempotency.Key)] = transaction.Idempotency
	}
	return cases, idempotency
}

func (s *Store) rebuildAfterProjectionMismatch(transactions []transactionEnvelope) error {
	s.cases = map[string]*domain.ConservationCase{}
	s.idempotency = map[string]domain.IdempotencyRecord{}
	for _, transaction := range transactions {
		s.cases[transaction.Case.ID] = transaction.Case.Clone()
		s.idempotency[idempotencyID(transaction.Idempotency.Scope, transaction.Idempotency.Key)] = transaction.Idempotency
	}
	s.sequence, s.headHash = int64(len(transactions)), latestHead(transactions)
	if len(transactions) > 0 {
		if err := s.writeSnapshot(); err != nil {
			return fmt.Errorf("重建投影: %w", err)
		}
	}
	return nil
}

func latestHead(transactions []transactionEnvelope) string {
	if len(transactions) == 0 {
		return ""
	}
	return transactions[len(transactions)-1].Hash
}
