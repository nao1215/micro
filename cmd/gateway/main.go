// API Gatewayサービスのエントリポイント。
// OAuth2認証（GitHub/Google）、JWT発行、リクエストルーティングを担当する。
// 外部からアクセス可能な唯一のサービスであり、セキュリティの境界線となる。
package main

import (
	"log"
	"os"

	"github.com/nao1215/micro/internal/gateway"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	server, err := gateway.NewServer(port)
	if err != nil {
		log.Fatalf("Gatewayサーバーの初期化に失敗: %v", err)
	}

	log.Printf("Gatewayサービスを起動します: :%s", port)
	if err := server.Run(); err != nil {
		log.Fatalf("Gatewayサービスの起動に失敗: %v", err)
	}
}
