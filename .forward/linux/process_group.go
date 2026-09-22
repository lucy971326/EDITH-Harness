// 运行：go run .forward/linux/process_group.go
package main

import (
	"fmt"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func main() {
	// 启动一个独立 Session 中的子进程。
	child := exec.Command("sleep", "10")
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := child.Start(); err != nil {
		fmt.Println("启动失败：", err)
		return
	}

	pid := child.Process.Pid
	group, _ := unix.Getpgid(pid)
	session, _ := unix.Getsid(pid)
	fmt.Printf("子进程 PID=%d，进程组 PGID=%d，Session SID=%d\n", pid, group, session)

	time.Sleep(time.Second)
	fmt.Println("向这个进程组发送 SIGTERM")
	if err := unix.Kill(-group, unix.SIGTERM); err != nil {
		fmt.Println("发送失败：", err)
		_ = child.Process.Kill()
	}

	// Wait 等子进程结束，并回收它。
	err := child.Wait()
	fmt.Printf("Wait 结果：%v，退出码：%d\n", err, child.ProcessState.ExitCode())
}
