package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"manuscript-conservation-gate/internal/audit"
	"manuscript-conservation-gate/internal/domain"
)

func appendEnvelope(path string, envelope transactionEnvelope) error {
	data, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("编码事件事务: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		return fmt.Errorf("打开事件日志: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
	}()
	if _, err := file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("追加事件日志: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("同步事件日志: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("关闭事件日志: %w", err)
	}
	closed = true
	return nil
}

func replayLog(path string) ([]transactionEnvelope, string, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return []transactionEnvelope{}, "", nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("打开事件日志: %w", err)
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	transactions := []transactionEnvelope{}
	previous := ""
	var expected int64 = 1
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			if line[len(line)-1] != '\n' {
				return nil, "", fmt.Errorf("%w: 事件日志末行不完整", domain.ErrIntegrity)
			}
			var envelope transactionEnvelope
			if err := json.Unmarshal(line, &envelope); err != nil {
				return nil, "", fmt.Errorf("%w: 无法解析事件日志第 %d 行", domain.ErrIntegrity, expected)
			}
			if envelope.SchemaVersion != schemaVersion {
				return nil, "", fmt.Errorf("%w: 不支持 schemaVersion=%d", domain.ErrIntegrity, envelope.SchemaVersion)
			}
			if envelope.Sequence != expected {
				return nil, "", fmt.Errorf("%w: 事件序号不连续", domain.ErrIntegrity)
			}
			if envelope.PreviousHash != previous {
				return nil, "", fmt.Errorf("%w: 事件链前序哈希不一致", domain.ErrIntegrity)
			}
			hash, err := envelopeHash(envelope)
			if err != nil || hash != envelope.Hash {
				return nil, "", fmt.Errorf("%w: 事件哈希不一致", domain.ErrIntegrity)
			}
			if envelope.Case == nil || envelope.Event.CaseID != envelope.Case.ID {
				return nil, "", fmt.Errorf("%w: 聚合与事件不匹配", domain.ErrIntegrity)
			}
			if err := audit.Verify(envelope.Case.AuditTrail); err != nil {
				return nil, "", err
			}
			if err := domain.ValidateAggregate(envelope.Case); err != nil {
				return nil, "", err
			}
			transactions = append(transactions, envelope)
			previous, expected = envelope.Hash, expected+1
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, "", fmt.Errorf("读取事件日志: %w", readErr)
		}
	}
	return transactions, previous, nil
}
