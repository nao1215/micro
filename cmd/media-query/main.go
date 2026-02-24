// メディアクエリサービスのエントリポイント。
// CQRSのQuery側を担当し、メディアの一覧・詳細・検索を処理する。
// Event Storeのイベントを購読してRead Model（SQLite）を構築・更新する。
package main

import (
	"log"
	"os"

	mediaquery "github.com/nao1215/micro/internal/media/query"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	server, err := mediaquery.NewServer(port)
	if err != nil {
		log.Fatalf("メディアクエリサーバーの初期化に失敗: %v", err)
	}

	log.Printf("メディアクエリサービスを起動します: :%s", port)
	if err := server.Run(); err != nil {
		log.Fatalf("メディアクエリサービスの起動に失敗: %v", err)
	}
}
