//go:build windows

package appserver

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

const (
	clsctxInprocServer = 0x1
	coinitApartment    = 0x2
	fosPickFolders     = 0x20
	fosForceFileSystem = 0x40
	sigdnFileSystem    = 0x80058000
	hresultCanceled    = 0x800704C7
	ifileDialogClose   = 23
)

var (
	clsidFileOpenDialog = windowsGUID{0xdc1c5a9c, 0xe88a, 0x4dde, [8]byte{0xa5, 0xa1, 0x60, 0xf8, 0x2a, 0x20, 0xae, 0xf7}}
	iidFileOpenDialog   = windowsGUID{0xd57c7288, 0xd4ad, 0x4768, [8]byte{0xbe, 0x02, 0x9d, 0x96, 0x95, 0x32, 0xd9, 0x60}}

	user32          = syscall.NewLazyDLL("user32.dll")
	setDialogTimer  = user32.NewProc("SetTimer")
	killDialogTimer = user32.NewProc("KillTimer")
	// 多个连接可各自在自己的 STA 线程打开弹窗；回调仅在安装定时器的线程执行。
	dialogTimers        sync.Map
	dialogTimerCallback = syscall.NewCallback(checkDialogCancellation)
)

type dialogCancellation struct {
	ctx    context.Context
	dialog *comObject
}

func checkDialogCancellation(_ uintptr, _ uintptr, timerID uintptr, _ uintptr) uintptr {
	value, ok := dialogTimers.Load(timerID)
	if !ok {
		return 0 // KillTimer 后残留的消息不能再访问已释放对象。
	}
	state := value.(dialogCancellation)
	if state.ctx.Err() != nil {
		// Show 的模态消息循环派发本回调；COM 调用始终留在所属 STA。
		// 若弹窗尚未就绪，后续 tick 继续尝试，不丢掉显示前发生的取消。
		comCall(state.dialog, ifileDialogClose, uintptr(hresultCanceled))
	}
	return 0
}

type windowsGUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

type comObject struct {
	vtable *[32]uintptr
}

func chooseWorkspace(ctx context.Context) (string, error) {
	err := ctx.Err()
	if err != nil {
		return "", err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	ole32 := syscall.NewLazyDLL("ole32.dll")
	coInitializeEx := ole32.NewProc("CoInitializeEx")
	coCreateInstance := ole32.NewProc("CoCreateInstance")
	coTaskMemFree := ole32.NewProc("CoTaskMemFree")

	hr, _, _ := coInitializeEx.Call(0, coinitApartment)
	if uint32(hr) != 0 && uint32(hr) != 1 {
		return "", fmt.Errorf("CoInitializeEx: 0x%x", uint32(hr))
	}
	defer ole32.NewProc("CoUninitialize").Call()

	var dialog *comObject
	hr, _, _ = coCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidFileOpenDialog)),
		0,
		clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidFileOpenDialog)),
		uintptr(unsafe.Pointer(&dialog)),
	)
	if uint32(hr) != 0 {
		return "", fmt.Errorf("CoCreateInstance(IFileOpenDialog): 0x%x", uint32(hr))
	}
	defer comRelease(dialog)

	hr = comCall(dialog, 9, fosPickFolders|fosForceFileSystem)
	if uint32(hr) != 0 {
		return "", fmt.Errorf("IFileOpenDialog.SetOptions: 0x%x", uint32(hr))
	}
	err = ctx.Err()
	if err != nil {
		return "", err
	}
	// 不创建 watcher 协程，也不跨线程传递 COM 指针。
	timerID, _, timerErr := setDialogTimer.Call(0, 0, 50, dialogTimerCallback)
	if timerID == 0 {
		return "", fmt.Errorf("SetTimer: %w", timerErr)
	}
	dialogTimers.Store(timerID, dialogCancellation{ctx: ctx, dialog: dialog})
	hr = comCall(dialog, 3, 0)
	// Show 返回后立即停止回调，先撤销访问权，再释放 COM 对象。
	dialogTimers.Delete(timerID)
	killDialogTimer.Call(0, timerID)
	err = ctx.Err()
	if err != nil {
		return "", err
	}
	if uint32(hr) == hresultCanceled {
		return "", errWorkspaceCanceled
	}
	if uint32(hr) != 0 {
		return "", fmt.Errorf("IFileOpenDialog.Show: 0x%x", uint32(hr))
	}
	var item *comObject
	hr = comCall(dialog, 20, uintptr(unsafe.Pointer(&item)))
	if uint32(hr) != 0 {
		return "", fmt.Errorf("IFileOpenDialog.GetResult: 0x%x", uint32(hr))
	}
	defer comRelease(item)
	var value *uint16
	hr = comCall(item, 5, sigdnFileSystem, uintptr(unsafe.Pointer(&value)))
	if uint32(hr) != 0 {
		return "", fmt.Errorf("IShellItem.GetDisplayName: 0x%x", uint32(hr))
	}
	defer coTaskMemFree.Call(uintptr(unsafe.Pointer(value)))
	return utf16String(value), nil
}

func comCall(object *comObject, index int, args ...uintptr) uintptr {
	r1, _, _ := syscall.SyscallN(object.vtable[index], append([]uintptr{uintptr(unsafe.Pointer(object))}, args...)...)
	return r1
}

func comRelease(object *comObject) {
	comCall(object, 2)
}

func utf16String(value *uint16) string {
	units := unsafe.Slice(value, 32768)
	for i, unit := range units {
		if unit == 0 {
			return syscall.UTF16ToString(units[:i])
		}
	}
	return ""
}
