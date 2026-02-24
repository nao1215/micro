package event

import (
	"encoding/json"
	"testing"
	"time"
)

// TestNew はNew関数でイベントが正しく生成されることを検証する。
func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("MediaUploadedDataでイベントを正常に生成できること", func(t *testing.T) {
		t.Parallel()

		data := MediaUploadedData{
			UserID:      "user-1",
			Filename:    "photo.jpg",
			ContentType: "image/jpeg",
			Size:        2048,
			StoragePath: "/uploads/photo.jpg",
		}

		before := time.Now().UTC()
		ev, err := New("media-1", AggregateTypeMedia, TypeMediaUploaded, 1, data)
		after := time.Now().UTC()

		if err != nil {
			t.Fatalf("New()でエラーが発生: %v", err)
		}
		if ev == nil {
			t.Fatal("New()がnilを返した")
		}

		// UUIDが生成されていること
		if ev.ID == "" {
			t.Error("IDが空文字列")
		}
		if ev.AggregateID != "media-1" {
			t.Errorf("AggregateID = %q, want %q", ev.AggregateID, "media-1")
		}
		if ev.AggregateType != AggregateTypeMedia {
			t.Errorf("AggregateType = %q, want %q", ev.AggregateType, AggregateTypeMedia)
		}
		if ev.EventType != TypeMediaUploaded {
			t.Errorf("EventType = %q, want %q", ev.EventType, TypeMediaUploaded)
		}
		if ev.Version != 1 {
			t.Errorf("Version = %d, want %d", ev.Version, 1)
		}

		// CreatedAtが呼び出し前後の範囲内であること
		if ev.CreatedAt.Before(before) || ev.CreatedAt.After(after) {
			t.Errorf("CreatedAt = %v, 期待する範囲: [%v, %v]", ev.CreatedAt, before, after)
		}

		// Dataが正しくシリアライズされていること
		var decoded MediaUploadedData
		if err := json.Unmarshal(ev.Data, &decoded); err != nil {
			t.Fatalf("Dataのデシリアライズに失敗: %v", err)
		}
		if decoded.UserID != data.UserID {
			t.Errorf("Data.UserID = %q, want %q", decoded.UserID, data.UserID)
		}
		if decoded.Filename != data.Filename {
			t.Errorf("Data.Filename = %q, want %q", decoded.Filename, data.Filename)
		}
	})

	t.Run("AlbumCreatedDataでイベントを正常に生成できること", func(t *testing.T) {
		t.Parallel()

		data := AlbumCreatedData{
			UserID:      "user-2",
			Name:        "テストアルバム",
			Description: "テスト用のアルバム",
		}

		ev, err := New("album-1", AggregateTypeAlbum, TypeAlbumCreated, 1, data)
		if err != nil {
			t.Fatalf("New()でエラーが発生: %v", err)
		}

		if ev.AggregateType != AggregateTypeAlbum {
			t.Errorf("AggregateType = %q, want %q", ev.AggregateType, AggregateTypeAlbum)
		}
		if ev.EventType != TypeAlbumCreated {
			t.Errorf("EventType = %q, want %q", ev.EventType, TypeAlbumCreated)
		}
	})

	t.Run("バージョン番号が正しく設定されること", func(t *testing.T) {
		t.Parallel()

		data := MediaDeletedData{UserID: "user-3"}

		ev, err := New("media-2", AggregateTypeMedia, TypeMediaDeleted, 42, data)
		if err != nil {
			t.Fatalf("New()でエラーが発生: %v", err)
		}

		if ev.Version != 42 {
			t.Errorf("Version = %d, want %d", ev.Version, 42)
		}
	})

	t.Run("連続して生成したイベントのIDが異なること", func(t *testing.T) {
		t.Parallel()

		data := MediaDeletedData{UserID: "user-4"}

		ev1, err := New("media-3", AggregateTypeMedia, TypeMediaDeleted, 1, data)
		if err != nil {
			t.Fatalf("1回目のNew()でエラーが発生: %v", err)
		}

		ev2, err := New("media-3", AggregateTypeMedia, TypeMediaDeleted, 2, data)
		if err != nil {
			t.Fatalf("2回目のNew()でエラーが発生: %v", err)
		}

		if ev1.ID == ev2.ID {
			t.Errorf("異なるイベントが同じIDを持っている: %q", ev1.ID)
		}
	})

	t.Run("シリアライズ不可能なデータでエラーが返ること", func(t *testing.T) {
		t.Parallel()

		// json.Marshalでエラーになるチャネル型を渡す
		invalidData := make(chan int)

		ev, err := New("media-4", AggregateTypeMedia, TypeMediaUploaded, 1, invalidData)
		if err == nil {
			t.Fatal("New()がエラーを返すべきだが、nilが返った")
		}
		if ev != nil {
			t.Error("エラー時にnilでないEventが返った")
		}
	})
}

// TestDecodeData はDecodeData関数でイベントデータを正しくデシリアライズできることを検証する。
func TestDecodeData(t *testing.T) {
	t.Parallel()

	t.Run("MediaUploadedDataを正しくデコードできること", func(t *testing.T) {
		t.Parallel()

		original := MediaUploadedData{
			UserID:      "user-10",
			Filename:    "video.mp4",
			ContentType: "video/mp4",
			Size:        5242880,
			StoragePath: "/uploads/video.mp4",
		}

		ev, err := New("media-10", AggregateTypeMedia, TypeMediaUploaded, 1, original)
		if err != nil {
			t.Fatalf("New()でエラーが発生: %v", err)
		}

		decoded, err := DecodeData[MediaUploadedData](ev)
		if err != nil {
			t.Fatalf("DecodeData()でエラーが発生: %v", err)
		}

		if decoded.UserID != original.UserID {
			t.Errorf("UserID = %q, want %q", decoded.UserID, original.UserID)
		}
		if decoded.Filename != original.Filename {
			t.Errorf("Filename = %q, want %q", decoded.Filename, original.Filename)
		}
		if decoded.ContentType != original.ContentType {
			t.Errorf("ContentType = %q, want %q", decoded.ContentType, original.ContentType)
		}
		if decoded.Size != original.Size {
			t.Errorf("Size = %d, want %d", decoded.Size, original.Size)
		}
		if decoded.StoragePath != original.StoragePath {
			t.Errorf("StoragePath = %q, want %q", decoded.StoragePath, original.StoragePath)
		}
	})

	t.Run("MediaProcessedDataを正しくデコードできること", func(t *testing.T) {
		t.Parallel()

		original := MediaProcessedData{
			ThumbnailPath:   "/thumbs/thumb.jpg",
			Width:           1920,
			Height:          1080,
			DurationSeconds: 30.5,
		}

		ev, err := New("media-11", AggregateTypeMedia, TypeMediaProcessed, 2, original)
		if err != nil {
			t.Fatalf("New()でエラーが発生: %v", err)
		}

		decoded, err := DecodeData[MediaProcessedData](ev)
		if err != nil {
			t.Fatalf("DecodeData()でエラーが発生: %v", err)
		}

		if decoded.ThumbnailPath != original.ThumbnailPath {
			t.Errorf("ThumbnailPath = %q, want %q", decoded.ThumbnailPath, original.ThumbnailPath)
		}
		if decoded.Width != original.Width {
			t.Errorf("Width = %d, want %d", decoded.Width, original.Width)
		}
		if decoded.Height != original.Height {
			t.Errorf("Height = %d, want %d", decoded.Height, original.Height)
		}
		if decoded.DurationSeconds != original.DurationSeconds {
			t.Errorf("DurationSeconds = %f, want %f", decoded.DurationSeconds, original.DurationSeconds)
		}
	})

	t.Run("NotificationSentDataを正しくデコードできること", func(t *testing.T) {
		t.Parallel()

		original := NotificationSentData{
			UserID:  "user-notify",
			Title:   "テスト通知",
			Message: "これはテスト通知です",
		}

		ev, err := New("user-notify", AggregateTypeUser, TypeNotificationSent, 1, original)
		if err != nil {
			t.Fatalf("New()でエラーが発生: %v", err)
		}

		decoded, err := DecodeData[NotificationSentData](ev)
		if err != nil {
			t.Fatalf("DecodeData()でエラーが発生: %v", err)
		}

		if decoded.UserID != original.UserID {
			t.Errorf("UserID = %q, want %q", decoded.UserID, original.UserID)
		}
		if decoded.Title != original.Title {
			t.Errorf("Title = %q, want %q", decoded.Title, original.Title)
		}
		if decoded.Message != original.Message {
			t.Errorf("Message = %q, want %q", decoded.Message, original.Message)
		}
	})

	t.Run("MediaUploadCompensatedDataを正しくデコードできること", func(t *testing.T) {
		t.Parallel()

		original := MediaUploadCompensatedData{
			Reason: "Sagaの補償処理",
			SagaID: "saga-100",
		}

		ev, err := New("media-comp", AggregateTypeMedia, TypeMediaUploadCompensated, 3, original)
		if err != nil {
			t.Fatalf("New()でエラーが発生: %v", err)
		}

		decoded, err := DecodeData[MediaUploadCompensatedData](ev)
		if err != nil {
			t.Fatalf("DecodeData()でエラーが発生: %v", err)
		}

		if decoded.Reason != original.Reason {
			t.Errorf("Reason = %q, want %q", decoded.Reason, original.Reason)
		}
		if decoded.SagaID != original.SagaID {
			t.Errorf("SagaID = %q, want %q", decoded.SagaID, original.SagaID)
		}
	})

	t.Run("不正なJSONデータでエラーが返ること", func(t *testing.T) {
		t.Parallel()

		ev := &Event{
			Data: json.RawMessage(`{invalid json`),
		}

		decoded, err := DecodeData[MediaUploadedData](ev)
		if err == nil {
			t.Fatal("DecodeData()がエラーを返すべきだが、nilが返った")
		}
		if decoded != nil {
			t.Error("エラー時にnilでないデータが返った")
		}
	})

	t.Run("空のJSONオブジェクトからデコードできること", func(t *testing.T) {
		t.Parallel()

		ev := &Event{
			Data: json.RawMessage(`{}`),
		}

		decoded, err := DecodeData[MediaUploadedData](ev)
		if err != nil {
			t.Fatalf("DecodeData()でエラーが発生: %v", err)
		}

		// ゼロ値であること
		if decoded.UserID != "" {
			t.Errorf("UserID = %q, want empty string", decoded.UserID)
		}
		if decoded.Size != 0 {
			t.Errorf("Size = %d, want 0", decoded.Size)
		}
	})

	t.Run("NewとDecodeDataのラウンドトリップが成功すること", func(t *testing.T) {
		t.Parallel()

		original := MediaAddedToAlbumData{
			MediaID: "media-roundtrip",
		}

		ev, err := New("album-rt", AggregateTypeAlbum, TypeMediaAddedToAlbum, 5, original)
		if err != nil {
			t.Fatalf("New()でエラーが発生: %v", err)
		}

		decoded, err := DecodeData[MediaAddedToAlbumData](ev)
		if err != nil {
			t.Fatalf("DecodeData()でエラーが発生: %v", err)
		}

		if decoded.MediaID != original.MediaID {
			t.Errorf("ラウンドトリップ後のMediaID = %q, want %q", decoded.MediaID, original.MediaID)
		}
	})
}
