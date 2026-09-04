package session

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// NewID 创建一个新的会话身份。
func NewID() (string, error) {
	var bytes [16]byte
	_, err := rand.Read(bytes[:])
	if err != nil {
		return "", fmt.Errorf("session: make id: %w", err)
	}
	return hex.EncodeToString(bytes[:]), nil
}
