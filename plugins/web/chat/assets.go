package chat

import (
	"embed"
	"fmt"
	"io/fs"
	nethttp "net/http"
)

// chatStaticFiles 保存仅属于 Chat 产品的浏览器资源。
//
//go:embed static/*
var chatStaticFiles embed.FS

func chatStaticHandler() (nethttp.Handler, error) {
	root, err := fs.Sub(chatStaticFiles, "static")
	if err != nil {
		return nil, fmt.Errorf("chat: static files: %w", err)
	}
	return nethttp.StripPrefix("/assets/chat/", nethttp.FileServerFS(root)), nil
}
