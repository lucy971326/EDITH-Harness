package web

import (
	"bytes"
	"fmt"
	"html"
	"net/http"

	"github.com/a-h/templ"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

var markdown = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
)

func render(writer http.ResponseWriter, request *http.Request, component templ.Component) {
	err := component.Render(request.Context(), writer)
	if err != nil {
		fmt.Printf("Web UI 渲染失败：%v\n", err)
	}
}

func renderMarkdown(source string) string {
	var output bytes.Buffer
	err := markdown.Convert([]byte(source), &output)
	if err != nil {
		return "<p>" + html.EscapeString(source) + "</p>"
	}
	return output.String()
}
