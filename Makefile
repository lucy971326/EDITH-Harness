.PHONY: run build test web

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
