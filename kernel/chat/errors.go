package chat

import "errors"

var (
	// ErrSessionSettings 标记读取目标会话设置失败。
	ErrSessionSettings = errors.New("chat service: session settings")
	// ErrRunStart 标记 Runner 未能启动新一轮。
	ErrRunStart = errors.New("chat service: start run")
	// ErrRunSteer 标记 Runner 未能接受 Steer。
	ErrRunSteer = errors.New("chat service: steer run")
)
