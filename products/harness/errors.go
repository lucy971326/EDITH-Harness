package harness

import "errors"

var (
	// ErrSessionNotFound 区分普通会话不存在与底层文件读取失败。
	ErrSessionNotFound = errors.New("harness product: session not found")
	// ErrWorkspace 标记创建输入中的工作区不可用。
	ErrWorkspace = errors.New("harness product: invalid workspace")
	// ErrSessionSettings 标记读取目标会话设置失败。
	ErrSessionSettings = errors.New("harness product: session settings")
	// ErrInvalidRunSettings 标记本轮选择的 Agent、模型或思考档位不合法。
	ErrInvalidRunSettings = errors.New("harness product: invalid run settings")
	// ErrRunStart 标记 Runner 未能启动新一轮。
	ErrRunStart = errors.New("harness product: start run")
	// ErrRunSteer 标记 Runner 未能接受 Steer。
	ErrRunSteer = errors.New("harness product: steer run")
)
