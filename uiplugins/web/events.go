package web

import (
	"fmt"
	"net/http"
)

func (s *service) handleEvents(writer http.ResponseWriter, request *http.Request) {
	sessionID := request.URL.Query().Get("session")
	if sessionID == "" {
		http.Error(writer, "缺少 session", http.StatusBadRequest)
		return
	}
	updates, unregister := s.updates.Subscribe(sessionID)
	defer unregister()
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Cache-Control", "no-cache")
	writer.Header().Set("Connection", "keep-alive")
	flusher, ok := writer.(http.Flusher)
	if !ok {
		http.Error(writer, "当前服务器不支持事件流", http.StatusInternalServerError)
		return
	}
	fmt.Fprint(writer, "event: ready\ndata: ok\n\n")
	flusher.Flush()
	for {
		select {
		case notice, open := <-updates:
			if !open {
				return
			}
			if notice.Chat {
				fmt.Fprint(writer, "event: refresh\ndata: now\n\n")
			}
			if notice.Composer {
				fmt.Fprint(writer, "event: composer\ndata: now\n\n")
			}
			flusher.Flush()
		case <-request.Context().Done():
			return
		}
	}
}
