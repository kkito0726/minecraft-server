# Minecraft サーバー管理コンソール（mcadmind）のビルドと検証。
#
#   make gen          proto から Go / TypeScript の型を生成する
#   make build        バックエンドとフロントエンドをビルドする
#   make build-arm64  Pi 5 用の静的バイナリを作る
#   make test         全テストを実行する
#   make verify       CI と同じ検証を一通り流す

SHELL := /bin/bash
.DEFAULT_GOAL := help

BIN_NAME    := mcadmind
BACKEND_DIR := backend
FRONT_DIR   := frontend
EMBED_DIR   := $(BACKEND_DIR)/internal/webui/dist
COVER_MIN   ?= 80
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS     := -s -w -X main.version=$(VERSION)

.PHONY: help
help: ## このヘルプを表示する
	@grep -E '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

# --- 依存 ---

.PHONY: deps
deps: ## 依存をインストールする
	cd $(BACKEND_DIR) && go mod download
	cd $(FRONT_DIR) && npm ci

# --- コード生成 ---

.PHONY: gen
gen: ## proto から Go / TypeScript の型を生成する
	buf lint
	buf generate

.PHONY: gen-check
gen-check: ## 生成物が proto と同期しているか確認する（CI 用）
	@$(MAKE) gen
	@if ! git diff --quiet -- $(BACKEND_DIR)/gen $(FRONT_DIR)/src/gen; then \
		echo "生成物が proto と同期していません。make gen を実行してコミットしてください。"; \
		git diff --stat -- $(BACKEND_DIR)/gen $(FRONT_DIR)/src/gen; \
		exit 1; \
	fi

# --- ビルド ---

# //go:embed all:dist は対象が 1 件も無いとコンパイルに失敗する。
# dist/ の中身は生成物なので Git では追跡せず、.gitkeep だけを追跡している。
# ところが vite build は emptyOutDir でその .gitkeep ごと消してしまうため、
# Go を触るすべてのターゲットの前でここを通して復元する。
# これを外すと「クローン直後に go build が通らない」状態に戻る。
.PHONY: ensure-embed
ensure-embed:
	@mkdir -p $(EMBED_DIR)
	@[ -e $(EMBED_DIR)/.gitkeep ] || touch $(EMBED_DIR)/.gitkeep

.PHONY: build
build: build-front build-back ## バックエンドとフロントエンドをビルドする

.PHONY: build-front
build-front: ## フロントエンドをビルドして Go の embed 先へ出す
	cd $(FRONT_DIR) && npm run build
	@$(MAKE) --no-print-directory ensure-embed

.PHONY: build-back
build-back: ensure-embed ## バックエンドをビルドする（ホストのアーキテクチャ向け）
	cd $(BACKEND_DIR) && go build -ldflags '$(LDFLAGS)' -o ../$(BIN_NAME) ./cmd/mcadmind

.PHONY: build-arm64
build-arm64: build-front ## Raspberry Pi 5 用の静的バイナリを作る
	cd $(BACKEND_DIR) && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
		go build -ldflags '$(LDFLAGS)' -o ../$(BIN_NAME)-arm64 ./cmd/mcadmind
	@file $(BIN_NAME)-arm64 2>/dev/null || true

# --- テスト ---

.PHONY: test
test: test-back test-front ## 全テストを実行する

.PHONY: test-back
test-back: ensure-embed ## Go のテストを実行する
	cd $(BACKEND_DIR) && go test -race ./...

.PHONY: test-integration
test-integration: ensure-embed ## Go の統合テストを実行する
	cd $(BACKEND_DIR) && go test -race -tags=integration ./...

.PHONY: test-front
test-front: ## フロントエンドのテストを実行する
	cd $(FRONT_DIR) && npm run test

.PHONY: cover
cover: ensure-embed ## Go のカバレッジを測る（生成物を除いて 80% 以上）
	cd $(BACKEND_DIR) && go test -race -coverprofile=../coverage.out -covermode=atomic ./...
	@grep -v '/gen/' coverage.out > coverage.filtered.out
	@mv coverage.filtered.out coverage.out
	@cd $(BACKEND_DIR) && go tool cover -func=../coverage.out | tail -1
	@total=$$(cd $(BACKEND_DIR) && go tool cover -func=../coverage.out | tail -1 | awk '{print $$NF}' | tr -d '%'); \
	 if [ -z "$$total" ]; then echo "カバレッジを取得できませんでした"; exit 1; fi; \
	 if awk "BEGIN{exit !($$total < $(COVER_MIN))}"; then \
		echo "カバレッジが $(COVER_MIN)% を下回っています: $$total%"; exit 1; \
	 else echo "カバレッジ OK: $$total%"; fi

.PHONY: cover-front
cover-front: ## フロントエンドのカバレッジを測る
	cd $(FRONT_DIR) && npm run test:coverage

.PHONY: e2e
e2e: build ## E2E テストを実行する
	cd $(FRONT_DIR) && npx playwright test

# --- 静的検査 ---

.PHONY: lint
lint: ensure-embed ## Go とフロントエンドの静的検査
	cd $(BACKEND_DIR) && gofmt -l . | (! grep .) || (echo "gofmt が必要です"; exit 1)
	cd $(BACKEND_DIR) && go vet ./...
	@# golangci-lint は CI で必ず走る。ローカルに無ければ案内だけ出して続行する
	@if command -v golangci-lint >/dev/null 2>&1; then \
		cd $(BACKEND_DIR) && golangci-lint run ./...; \
	else \
		echo "golangci-lint が未導入のためスキップします（brew install golangci-lint）"; \
		echo "CI では実行されるので、push 前に入れておくことを推奨します"; \
	fi
	cd $(FRONT_DIR) && npm run lint

.PHONY: typecheck
typecheck: ## TypeScript の型検査
	cd $(FRONT_DIR) && npm run typecheck

.PHONY: fmt
fmt: ## コードを整形する
	cd $(BACKEND_DIR) && go fmt ./...
	buf format -w

.PHONY: verify
verify: gen-check lint typecheck test cover ## CI と同じ検証を一通り流す

# --- 後始末 ---

.PHONY: clean
clean: ## ビルド成果物を削除する
	rm -f $(BIN_NAME) $(BIN_NAME)-arm64 coverage.out coverage.html
	rm -rf $(EMBED_DIR)/assets $(EMBED_DIR)/index.html $(EMBED_DIR)/vite.svg
	rm -rf $(FRONT_DIR)/dist $(FRONT_DIR)/coverage
