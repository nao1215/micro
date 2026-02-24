package command

import (
	"fmt"
	"os"
)

// mediaBaseDir はメディアファイルの保存先ベースディレクトリ。
// テスト時に差し替え可能にするためvarとして宣言する。
var mediaBaseDir = "/data/media"

// initStorage はメディアファイルの保存先ディレクトリを作成する。
// ディレクトリが既に存在する場合は何もしない。
func initStorage() error {
	if err := os.MkdirAll(mediaBaseDir, 0o755); err != nil {
		return fmt.Errorf("メディア保存ディレクトリの作成に失敗: %w", err)
	}
	return nil
}
