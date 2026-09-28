# Harness 开发命令入口。日常命令都在这里；Desktop 构建钩子在 Taskfile.yml（wails3 约定，不回调本文件）。
# 本机 LLM 密钥在 ~/.harness/config.yaml（用户数据，与构建无关）。

# Web：开发模式由 Vite 热更新页面，Go 后台提供 /rpc。
.PHONY: web run run-backend run-frontend build

web:
	npm --prefix clients ci
	npm --prefix clients run build

run: clients/node_modules/.package-lock.json clients/dist/index.html
	@$(MAKE) --no-print-directory -j2 run-backend run-frontend

clients/dist/index.html: clients/node_modules/.package-lock.json
	npm --prefix clients run build

run-backend:
	go run ./cmd/harness --no-browser

run-frontend:
	npm --prefix clients run dev -- --open --strictPort

build: web
	mkdir -p .build
	go build -o .build/harness ./cmd/harness

# Desktop：Wails 开发与构建（Taskfile.yml 承接构建细节）
.PHONY: desktop-run desktop-build desktop-package

desktop-run:
	wails3 dev

desktop-build:
	wails3 build

desktop-package:
	wails3 task package

# 验收：日常快速检查 / 发布前完整检查
.PHONY: agent-check test

# reference 仅供阅读，复制的 SDK 源码不属于 Harness Go module 的检查目标。
GO_PACKAGES := ./clients/... ./cmd/... ./internal/... ./tests/...

# Agent 日常快速回归：依赖按需安装，页面只构建一次，其余检查并行执行。
# 发布前或干净环境的最终验收仍使用 make test。
agent-check: agent-web-build
	@$(MAKE) --no-print-directory -j4 agent-go agent-race agent-contracts agent-web-test

test: web
	go test $(GO_PACKAGES)
	go vet $(GO_PACKAGES)
	go test -race ./clients/web ./internal/appserver ./internal/conversations ./internal/runner
	npm --prefix clients run contracts:check
	npm --prefix clients run rpc:check
	npm --prefix clients run rpc:test
	npm --prefix clients test

# agent-check 的子任务
.PHONY: agent-web-build agent-go agent-race agent-contracts agent-web-test

agent-web-build: clients/node_modules/.package-lock.json
	npm --prefix clients run build

agent-go:
	go test $(GO_PACKAGES)
	go vet $(GO_PACKAGES)

agent-race:
	go test -race ./clients/web ./internal/appserver ./internal/conversations ./internal/runner ./internal/subagents ./internal/machine/local ./internal/tools/subagents

agent-contracts: clients/node_modules/.package-lock.json
	npm --prefix clients run contracts:check
	npm --prefix clients run rpc:check

agent-web-test: clients/node_modules/.package-lock.json
	npm --prefix clients test

# 按需安装 npm 依赖
clients/node_modules/.package-lock.json: clients/package.json clients/package-lock.json
	npm --prefix clients ci --no-audit --no-fund
