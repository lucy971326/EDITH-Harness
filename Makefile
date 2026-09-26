# Harness 开发命令入口。日常命令都在这里；Desktop 构建钩子在 Taskfile.yml（wails3 约定，不回调本文件）。
# 本机 LLM 密钥在 ~/.harness/config.yaml（用户数据，与构建无关）。

# Web：运行与构建
.PHONY: web run build

web:
	npm --prefix clients/web ci
	npm --prefix clients/web run build

run: web
	go run ./cmd/harness

build: web
	mkdir -p .build
	go build -o .build/harness ./cmd/harness

# Desktop：Wails 开发与构建（Taskfile.yml 承接构建细节）
.PHONY: desktop-run desktop-build

desktop-run:
	wails3 dev

desktop-build:
	wails3 build

# 验收：日常快速检查 / 发布前完整检查
.PHONY: agent-check test

# Agent 日常快速回归：依赖按需安装，页面只构建一次，其余检查并行执行。
# 发布前或干净环境的最终验收仍使用 make test。
agent-check: agent-web-build
	@$(MAKE) --no-print-directory -j4 agent-go agent-race agent-contracts agent-web-test

test: web
	npm --prefix clients ci
	go test ./...
	go vet ./...
	go test -race ./internal/appserver ./internal/conversations ./internal/runner
	npm --prefix clients run contracts:check
	npm --prefix clients run rpc:check
	npm --prefix clients run rpc:test
	npm --prefix clients/web test

# agent-check 的子任务
.PHONY: agent-web-build agent-go agent-race agent-contracts agent-web-test

agent-web-build: clients/web/node_modules/.package-lock.json
	npm --prefix clients/web run build

agent-go:
	go test ./...
	go vet ./...

agent-race:
	go test -race ./internal/appserver ./internal/conversations ./internal/runner ./internal/subagents ./internal/machine/local ./internal/tools/subagents

agent-contracts: clients/node_modules/.package-lock.json
	npm --prefix clients run contracts:check
	npm --prefix clients run rpc:check

agent-web-test: clients/web/node_modules/.package-lock.json
	npm --prefix clients/web test

# 按需安装 npm 依赖
clients/node_modules/.package-lock.json: clients/package.json clients/package-lock.json
	npm --prefix clients ci --no-audit --no-fund

clients/web/node_modules/.package-lock.json: clients/web/package.json clients/web/package-lock.json
	npm --prefix clients/web ci --no-audit --no-fund
