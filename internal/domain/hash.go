package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

func HashJSON(value any) (string, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
