//go:build !windows

package mcp

// Unix 凭据文件由 persist 以 0600 原子替换，目录以 0700 创建。
func sealOAuthCredential(data []byte) ([]byte, error) { return data, nil }
func openOAuthCredential(data []byte) ([]byte, error) { return data, nil }
