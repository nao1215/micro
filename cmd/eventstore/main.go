// イベントストアサービスのエントリポイント。
// すべてのサービスの状態変更をイベントとして永続化し、配信する。
// Event Sourcingの中核となるサービスであり、唯一の「真実の源泉」として機能する。
package main

import (
	"log"
	"os"

	"github.com/nao1215/micro/internal/eventstore"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8084"
	}

	server, err := eventstore.NewServer(port)
	if err != nil {
		log.Fatalf("イベントストアサーバーの初期化に失敗: %v", err)
	}

	log.Printf("イベントストアサービスを起動します: :%s", port)
	if err := server.Run(); err != nil {
		log.Fatalf("イベントストアサービスの起動に失敗: %v", err)
	}
}
