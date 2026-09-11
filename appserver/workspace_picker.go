package appserver

import "errors"

// errWorkspaceCanceled 表示用户主动取消原生目录选择。
var errWorkspaceCanceled = errors.New("appserver: workspace selection canceled")

// selectWorkspace 是本机目录选择入口；测试可替换，不经过 Product。
var selectWorkspace = chooseWorkspace
