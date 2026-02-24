// 通知サービスのエントリポイント。
// イベント駆動で通知を配信する。メディア処理完了やSaga完了時に
// ユーザーへの通知を生成・保存する。
package main

import (
	"log"
	"os"

	"github.com/nao1215/micro/internal/notification"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8086"
	}

	server, err := notification.NewServer(port)
	if err != nil {
		log.Fatalf("通知サーバーの初期化に失敗: %v", err)
	}

	log.Printf("通知サービスを起動します: :%s", port)
	if err := server.Run(); err != nil {
		log.Fatalf("通知サービスの起動に失敗: %v", err)
	}
}
