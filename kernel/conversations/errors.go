package conversations

import "errors"

var (
	// ErrRunChanged 标记定向插话的目标已结束、换轮或不再接受输入。
	ErrRunChanged = errors.New("conversation: expected run changed")
	// ErrInvalidMessage 标记空消息或不可用的消息内容。
	ErrInvalidMessage = errors.New("conversation: invalid message")
	// ErrSessionNotFound 区分普通会话不存在与底层文件读取失败。
	ErrSessionNotFound = errors.New("conversation: session not found")
	// ErrWorkspace 标记创建输入中的工作区不可用。
	ErrWorkspace = errors.New("conversation: invalid workspace")
	// ErrSessionSettings 标记读取目标会话设置失败。
	ErrSessionSettings = errors.New("conversation: session settings")
	// ErrInvalidRunSettings 标记本轮选择的 Agent、模型或思考档位不合法。
	ErrInvalidRunSettings = errors.New("conversation: invalid run settings")
	// ErrRunActive 标记运行中的会话不接受当前操作。
	ErrRunActive = errors.New("conversation: run is active")
	// ErrRunStart 标记 Runner 未能启动新一轮。
	ErrRunStart = errors.New("conversation: start run")
	// ErrRunSteer 标记 Runner 未能接受 Steer。
	ErrRunSteer = errors.New("conversation: steer run")
	// ErrInvalidCommand 标记请求的产品命令不存在。
	ErrInvalidCommand = errors.New("conversation: invalid command")
	// ErrCommandRejected 标记命令因当前会话状态不能被接受。
	ErrCommandRejected = errors.New("conversation: command rejected")
)
