// アルバムサービスのエントリポイント。
// アルバムのCRUDとメディアとの関連付けを管理する。
// メディアアップロードSagaの一部として、自動的にデフォルトアルバムへの追加も行う。
package main

import (
	"log"
	"os"

	"github.com/nao1215/micro/internal/album"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8083"
	}

	server, err := album.NewServer(port)
	if err != nil {
		log.Fatalf("アルバムサーバーの初期化に失敗: %v", err)
	}

	log.Printf("アルバムサービスを起動します: :%s", port)
	if err := server.Run(); err != nil {
		log.Fatalf("アルバムサービスの起動に失敗: %v", err)
	}
}
