package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"manuscript-conservation-gate/internal/domain"
)

func (s *Store) writeSnapshot() error {
	projection := projectionFile{SchemaVersion: schemaVersion, Sequence: s.sequence, LogHeadHash: s.headHash, Cases: s.cases, Idempotency: s.idempotency}
	data, err := json.MarshalIndent(projection, "", "  ")
	if err != nil {
		return fmt.Errorf("编码投影: %w", err)
	}
	temporary, err := os.CreateTemp(s.dir, ".projection-*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时投影: %w", err)
	}
	tempName := temporary.Name()
	defer os.Remove(tempName)
	if err := temporary.Chmod(0o640); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("写入临时投影: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("同步临时投影: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("关闭临时投影: %w", err)
	}
	if err := os.Rename(tempName, s.snapshotPath); err != nil {
		return fmt.Errorf("原子替换投影: %w", err)
	}
	directory, err := os.Open(filepath.Dir(s.snapshotPath))
	if err != nil {
		return fmt.Errorf("打开投影目录: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("同步投影目录: %w", err)
	}
	return nil
}

func readSnapshot(path string) (projectionFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return projectionFile{}, err
	}
	var projection projectionFile
	if err := json.Unmarshal(data, &projection); err != nil {
		return projectionFile{}, fmt.Errorf("解析投影: %w", err)
	}
	if projection.SchemaVersion != schemaVersion {
		return projectionFile{}, fmt.Errorf("%w: 投影 schemaVersion 不兼容", domain.ErrIntegrity)
	}
	if projection.Cases == nil || projection.Idempotency == nil {
		return projectionFile{}, fmt.Errorf("%w: 投影缺少必要集合", domain.ErrIntegrity)
	}
	return projection, nil
}
