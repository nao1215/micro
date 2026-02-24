// Sagaオーケストレータサービスのエントリポイント。
// 分散トランザクションの調整と失敗時の補償アクション管理を担当する。
// Event Storeからイベントを購読し、Sagaの各ステップを順次実行する。
// 失敗時には逆順に補償アクションを実行して整合性を保つ。
package main

import (
	"log"
	"os"

	"github.com/nao1215/micro/internal/saga"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8085"
	}

	server, err := saga.NewServer(port)
	if err != nil {
		log.Fatalf("Sagaサーバーの初期化に失敗: %v", err)
	}

	log.Printf("Sagaサービスを起動します: :%s", port)
	if err := server.Run(); err != nil {
		log.Fatalf("Sagaサービスの起動に失敗: %v", err)
	}
}
