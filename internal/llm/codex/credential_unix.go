//go:build !windows

package codex

// Unix 凭据文件由 persist 以 0600 原子替换，目录以 0700 创建。
func sealOAuthCredential(data []byte) ([]byte, error) { return append([]byte(nil), data...), nil }
func openOAuthCredential(data []byte) ([]byte, error) { return data, nil }
