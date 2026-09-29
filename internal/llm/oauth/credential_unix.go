//go:build !windows

package oauth

// Unix 凭据文件由 persist 以 0600 原子替换，目录以 0700 创建。
func sealCredential(data []byte) ([]byte, error) { return append([]byte(nil), data...), nil }
func openCredential(data []byte) ([]byte, error) { return data, nil }
