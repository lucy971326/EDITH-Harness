//go:build windows

package oauth

import (
	"errors"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows 凭据通过 DPAPI 绑定当前用户；文件仍由 persist 原子写入。
func sealCredential(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty OAuth credential")
	}
	input := windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
	var output windows.DataBlob
	err := windows.CryptProtectData(&input, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &output)
	runtime.KeepAlive(data)
	if err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(output.Data)))
	return append([]byte(nil), unsafe.Slice(output.Data, int(output.Size))...), nil
}

func openCredential(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty OAuth credential")
	}
	input := windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
	var output windows.DataBlob
	err := windows.CryptUnprotectData(&input, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &output)
	runtime.KeepAlive(data)
	if err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(output.Data)))
	plain := unsafe.Slice(output.Data, int(output.Size))
	copyOfPlain := append([]byte(nil), plain...)
	clear(plain)
	return copyOfPlain, nil
}
