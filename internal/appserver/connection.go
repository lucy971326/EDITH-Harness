package appserver

import (
	"harness/internal/appserver/internal/clientconn"

	"github.com/sourcegraph/jsonrpc2"
)

const MaxRPCMessageBytes = 16 << 20

// ServeStream 为所有传输提供相同的初始化、请求与订阅生命周期。
func (s *Server) ServeStream(stream jsonrpc2.ObjectStream) {
	if !s.lifecycle.begin() {
		stream.Close()
		return
	}
	defer s.lifecycle.end()
	connection := clientconn.New(s.lifecycle.context(), stream, s.prepareCall)
	connection.Run()
}

// Close 拒绝新调用，断开 Client 并等待请求结束；不停止已接受的 Run。
func (s *Server) Close() error {
	s.closeOnce.Do(func() {
		s.lifecycle.close()
		s.lifecycle.wait()
	})
	return nil
}
