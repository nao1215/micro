.PHONY: build test lint generate docker-up docker-down clean tools help

# サービス一覧
SERVICES := gateway media-command media-query album eventstore saga notification

# ビルド出力先
BIN_DIR := bin

help: ## ヘルプを表示
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## 全サービスをビルド
	@mkdir -p $(BIN_DIR)
	@for svc in $(SERVICES); do \
		echo "Building $$svc..."; \
		go build -o $(BIN_DIR)/$$svc ./cmd/$$svc/; \
	done

test: ## テスト実行とカバレッジ計測
	go test -v -race -coverprofile=cover.out ./...
	go tool cover -html=cover.out -o cover.html

lint: ## golangci-lintによるコード検査
	golangci-lint run ./...

generate: ## sqlcコード生成
	@for dir in db/*/; do \
		if [ -f "$$dir/sqlc.yaml" ]; then \
			echo "Generating sqlc for $$dir..."; \
			(cd $$dir && sqlc generate); \
		fi; \
	done

docker-up: ## Docker Composeで全サービス起動
	docker compose up --build -d

docker-down: ## Docker Composeで全サービス停止
	docker compose down

docker-logs: ## Docker Composeのログを表示
	docker compose logs -f

clean: ## 生成ファイル削除
	rm -rf $(BIN_DIR)
	rm -f cover.out cover.html
	find . -name "*.db" -path "*/testdata/*" -prune -o -name "*.db" -print | xargs rm -f 2>/dev/null || true

tools: ## 開発ツールのインストール
	go install github.com/golangci-lint/golangci-lint/v2/cmd/golangci-lint@latest
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
	go install github.com/k1LoW/octocov@latest

fmt: ## コードフォーマット
	go fmt ./...
	goimports -w .
