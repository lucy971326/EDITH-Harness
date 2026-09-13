.PHONY: run build test web agent-check agent-web-build agent-go agent-race agent-contracts agent-web-test

web:
	npm --prefix clients/web ci
	npm --prefix clients/web run build

run: web
	go run ./cmd/harness

build: web
	mkdir -p .build
	go build -o .build/harness ./cmd/harness

test: web
	npm --prefix clients ci
	go test ./...
	go vet ./...
	go test -race ./appserver ./products/harness ./kernel/runner
	npm --prefix clients run contracts:check
	npm --prefix clients run rpc:check
	npm --prefix clients run rpc:test
	npm --prefix clients/web test

# Agent 日常快速回归：依赖按需安装，页面只构建一次，其余检查并行执行。
# 发布前或干净环境的最终验收仍使用 make test。
agent-check: agent-web-build
	@$(MAKE) --no-print-directory -j4 agent-go agent-race agent-contracts agent-web-test

agent-web-build: clients/web/node_modules/.package-lock.json
	npm --prefix clients/web run build

agent-go:
	go test ./...
	go vet ./...

agent-race:
	go test -race ./appserver ./products/harness ./kernel/runner ./kernel/subagents ./plugins/tools/subagents

agent-contracts: clients/node_modules/.package-lock.json
	npm --prefix clients run contracts:check
	npm --prefix clients run rpc:check

agent-web-test: clients/web/node_modules/.package-lock.json
	npm --prefix clients/web test

clients/node_modules/.package-lock.json: clients/package.json clients/package-lock.json
	npm --prefix clients ci --no-audit --no-fund

clients/web/node_modules/.package-lock.json: clients/web/package.json clients/web/package-lock.json
	npm --prefix clients/web ci --no-audit --no-fund
