package chat

import (
	"encoding/base64"
	"fmt"
	"io"
	nethttp "net/http"
	"strings"

	"harness/kernel/session"
)

var allowedImageMIME = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/gif":  true,
	"image/webp": true,
}

func userMessageFromRequest(r *nethttp.Request) (session.UserMessage, error) {
	text := strings.TrimSpace(r.FormValue("text"))
	images, err := readImageBlocks(r)
	if err != nil {
		return session.UserMessage{}, err
	}
	if text == "" && len(images) == 0 {
		return session.UserMessage{}, fmt.Errorf("请输入文字或图片")
	}
	blocks := images
	if text != "" {
		blocks = append(blocks, session.Block{Kind: "text", Text: text})
	}
	return session.UserMessage{Blocks: blocks}, nil
}

func hasImages(input session.UserMessage) bool {
	for _, block := range input.Blocks {
		if block.Kind == "image" {
			return true
		}
	}
	return false
}

func readImageBlocks(r *nethttp.Request) ([]session.Block, error) {
	if r.MultipartForm == nil {
		return nil, nil
	}
	headers := r.MultipartForm.File["images"]
	if len(headers) == 0 {
		return nil, nil
	}
	out := make([]session.Block, 0, len(headers))
	for _, header := range headers {
		file, err := header.Open()
		if err != nil {
			return nil, fmt.Errorf("读取图片失败")
		}
		data, err := io.ReadAll(file)
		closeErr := file.Close()
		if err != nil {
			return nil, fmt.Errorf("读取图片失败")
		}
		if closeErr != nil {
			return nil, fmt.Errorf("读取图片失败")
		}
		mime := imageMIME(data, header.Header.Get("Content-Type"))
		if mime == "" {
			return nil, fmt.Errorf("不支持的图片类型")
		}
		out = append(out, session.Block{
			Kind: "image",
			Media: &session.Media{
				MIME: mime,
				Data: base64.StdEncoding.EncodeToString(data),
			},
		})
	}
	return out, nil
}

func imageMIME(data []byte, declared string) string {
	detected := nethttp.DetectContentType(data)
	if allowedImageMIME[detected] {
		return detected
	}
	declared = strings.TrimSpace(declared)
	if allowedImageMIME[declared] {
		return declared
	}
	return ""
}
