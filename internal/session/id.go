package session

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// NewID 创建一个新的会话身份。
func NewID() (string, error) {
	return randomID("session: make id")
}

// NewEntryID 提前分配一条账本消息的稳定身份；此时并不落账，也不占用 Seq。
func NewEntryID() (string, error) {
	return randomID("session: make entry id")
}

func randomID(op string) (string, error) {
	var bytes [16]byte
	_, err := rand.Read(bytes[:])
	if err != nil {
		return "", fmt.Errorf("%s: %w", op, err)
	}
	return hex.EncodeToString(bytes[:]), nil
}
