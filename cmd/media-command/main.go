// メディアコマンドサービスのエントリポイント。
// CQRSのCommand側を担当し、メディアのアップロード・更新・削除を処理する。
// ファイルの保存とサムネイル生成を行い、イベントをEvent Storeに発行する。
package main

import (
	"log"
	"os"

	mediacommand "github.com/nao1215/micro/internal/media/command"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	server, err := mediacommand.NewServer(port)
	if err != nil {
		log.Fatalf("メディアコマンドサーバーの初期化に失敗: %v", err)
	}

	log.Printf("メディアコマンドサービスを起動します: :%s", port)
	if err := server.Run(); err != nil {
		log.Fatalf("メディアコマンドサービスの起動に失敗: %v", err)
	}
}
