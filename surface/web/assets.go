package web

import "embed"

// staticFiles 保存构建时编译或固定版本的浏览器资源。
//
//go:embed static/*
var staticFiles embed.FS
