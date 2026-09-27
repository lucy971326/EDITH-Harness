package desktop

import (
	"encoding/json"
	"fmt"
	"time"

	"harness/internal/appserver"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// 活对象。Stream 只转换一帧完整的 JSON-RPC 消息，不处理业务方法。
type Stream struct {
	Conn *application.StreamConn
}

func (s *Stream) ReadObject(value any) error {
	raw, err := s.Conn.Receive()
	if err != nil {
		return err
	}
	if len(raw) > appserver.MaxRPCMessageBytes {
		return fmt.Errorf("desktop: RPC message exceeds 16 MiB")
	}
	return json.Unmarshal(raw, value)
}

func (s *Stream) WriteObject(value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(raw) > appserver.MaxRPCMessageBytes {
		return fmt.Errorf("desktop: RPC message exceeds 16 MiB")
	}
	// Wails 的 Send 在窗口不消费时可能阻塞，限时关闭会唤醒它。
	timer := time.AfterFunc(5*time.Second, func() { s.Conn.Close() })
	defer timer.Stop()
	return s.Conn.Send(raw)
}

func (s *Stream) Close() error {
	return s.Conn.Close()
}
