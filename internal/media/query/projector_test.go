package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	mediadb "github.com/nao1215/micro/internal/media/query/db"
	"github.com/nao1215/micro/pkg/event"
)

// setupTestProjector はテスト用のProjectorとインメモリSQLiteを作成する。
// Event StoreのURLは空でよい（processEventのテストではHTTP通信を行わないため）。
func setupTestProjector(t *testing.T) (*Projector, *mediadb.Queries, *sql.DB) {
	t.Helper()

	sqlDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("インメモリSQLiteの接続に失敗: %v", err)
	}

	if err := initSchema(sqlDB); err != nil {
		t.Fatalf("Read Modelスキーマの初期化に失敗: %v", err)
	}

	queries := mediadb.New(sqlDB)
	projector := NewProjector(queries, "http://localhost:9999")

	t.Cleanup(func() {
		sqlDB.Close()
	})

	return projector, queries, sqlDB
}

// makeEventJSON はイベントデータ構造体をJSON文字列に変換する。
func makeEventJSON(t *testing.T, data any) string {
	t.Helper()
	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("イベントデータのシリアライズに失敗: %v", err)
	}
	return string(b)
}

func TestProcessEvent_MediaUploaded(t *testing.T) {
	t.Parallel()

	t.Run("正常系_MediaUploadedイベントでRead Modelにレコードが挿入される", func(t *testing.T) {
		t.Parallel()

		p, queries, _ := setupTestProjector(t)
		ctx := context.Background()

		uploadedData := event.MediaUploadedData{
			UserID:      "user-123",
			Filename:    "test_photo.jpg",
			ContentType: "image/jpeg",
			Size:        4096,
			StoragePath: "/data/media/media-upload-1/test_photo.jpg",
		}

		ev := eventStoreResponse{
			ID:            "event-1",
			AggregateID:   "media-upload-1",
			AggregateType: string(event.AggregateTypeMedia),
			EventType:     string(event.TypeMediaUploaded),
			Data:          makeEventJSON(t, uploadedData),
			Version:       1,
			CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		}

		if err := p.processEvent(ctx, ev); err != nil {
			t.Fatalf("processEventが失敗: %v", err)
		}

		// Read Modelからレコードを取得して検証する
		model, err := queries.GetMediaByID(ctx, "media-upload-1")
		if err != nil {
			t.Fatalf("GetMediaByIDが失敗: %v", err)
		}

		if model.ID != "media-upload-1" {
			t.Errorf("期待するID %q, 実際のID %q", "media-upload-1", model.ID)
		}
		if model.UserID != "user-123" {
			t.Errorf("期待するUserID %q, 実際のUserID %q", "user-123", model.UserID)
		}
		if model.Filename != "test_photo.jpg" {
			t.Errorf("期待するFilename %q, 実際のFilename %q", "test_photo.jpg", model.Filename)
		}
		if model.ContentType != "image/jpeg" {
			t.Errorf("期待するContentType %q, 実際のContentType %q", "image/jpeg", model.ContentType)
		}
		if model.Size != 4096 {
			t.Errorf("期待するSize %d, 実際のSize %d", 4096, model.Size)
		}
		if model.StoragePath != "/data/media/media-upload-1/test_photo.jpg" {
			t.Errorf("期待するStoragePath %q, 実際のStoragePath %q", "/data/media/media-upload-1/test_photo.jpg", model.StoragePath)
		}
		if model.Status != "uploaded" {
			t.Errorf("期待するStatus %q, 実際のStatus %q", "uploaded", model.Status)
		}
		if model.LastEventVersion != 1 {
			t.Errorf("期待するLastEventVersion %d, 実際のLastEventVersion %d", 1, model.LastEventVersion)
		}
	})
}

func TestProcessEvent_MediaProcessed(t *testing.T) {
	t.Parallel()

	t.Run("正常系_MediaProcessedイベントでサムネイル情報が更新される", func(t *testing.T) {
		t.Parallel()

		p, queries, _ := setupTestProjector(t)
		ctx := context.Background()

		// 先にMediaUploadedでレコードを作成する
		uploadedData := event.MediaUploadedData{
			UserID:      "user-123",
			Filename:    "photo.jpg",
			ContentType: "image/jpeg",
			Size:        8192,
			StoragePath: "/data/media/media-proc-1/photo.jpg",
		}
		uploadEv := eventStoreResponse{
			ID:            "event-1",
			AggregateID:   "media-proc-1",
			AggregateType: string(event.AggregateTypeMedia),
			EventType:     string(event.TypeMediaUploaded),
			Data:          makeEventJSON(t, uploadedData),
			Version:       1,
			CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		}
		if err := p.processEvent(ctx, uploadEv); err != nil {
			t.Fatalf("MediaUploadedの処理に失敗: %v", err)
		}

		// MediaProcessedイベントを処理する
		processedData := event.MediaProcessedData{
			ThumbnailPath: "/data/media/media-proc-1/thumbnail.jpg",
			Width:         1920,
			Height:        1080,
		}
		processEv := eventStoreResponse{
			ID:            "event-2",
			AggregateID:   "media-proc-1",
			AggregateType: string(event.AggregateTypeMedia),
			EventType:     string(event.TypeMediaProcessed),
			Data:          makeEventJSON(t, processedData),
			Version:       2,
			CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		}
		if err := p.processEvent(ctx, processEv); err != nil {
			t.Fatalf("MediaProcessedの処理に失敗: %v", err)
		}

		// Read Modelを検証する
		model, err := queries.GetMediaByID(ctx, "media-proc-1")
		if err != nil {
			t.Fatalf("GetMediaByIDが失敗: %v", err)
		}

		if model.Status != "processed" {
			t.Errorf("期待するStatus %q, 実際のStatus %q", "processed", model.Status)
		}
		if !model.ThumbnailPath.Valid || model.ThumbnailPath.String != "/data/media/media-proc-1/thumbnail.jpg" {
			t.Errorf("期待するThumbnailPath %q, 実際のThumbnailPath %v", "/data/media/media-proc-1/thumbnail.jpg", model.ThumbnailPath)
		}
		if !model.Width.Valid || model.Width.Int64 != 1920 {
			t.Errorf("期待するWidth 1920, 実際のWidth %v", model.Width)
		}
		if !model.Height.Valid || model.Height.Int64 != 1080 {
			t.Errorf("期待するHeight 1080, 実際のHeight %v", model.Height)
		}
		if model.LastEventVersion != 2 {
			t.Errorf("期待するLastEventVersion 2, 実際のLastEventVersion %d", model.LastEventVersion)
		}
	})
}

func TestProcessEvent_MediaProcessingFailed(t *testing.T) {
	t.Parallel()

	t.Run("正常系_MediaProcessingFailedイベントでステータスがfailedになる", func(t *testing.T) {
		t.Parallel()

		p, queries, _ := setupTestProjector(t)
		ctx := context.Background()

		// 先にMediaUploadedでレコードを作成する
		uploadedData := event.MediaUploadedData{
			UserID:      "user-123",
			Filename:    "broken.jpg",
			ContentType: "image/jpeg",
			Size:        1024,
			StoragePath: "/data/media/media-fail-1/broken.jpg",
		}
		uploadEv := eventStoreResponse{
			ID:            "event-1",
			AggregateID:   "media-fail-1",
			AggregateType: string(event.AggregateTypeMedia),
			EventType:     string(event.TypeMediaUploaded),
			Data:          makeEventJSON(t, uploadedData),
			Version:       1,
			CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		}
		if err := p.processEvent(ctx, uploadEv); err != nil {
			t.Fatalf("MediaUploadedの処理に失敗: %v", err)
		}

		// MediaProcessingFailedイベントを処理する
		failedData := event.MediaProcessingFailedData{
			Reason: "画像のデコードに失敗しました",
		}
		failEv := eventStoreResponse{
			ID:            "event-2",
			AggregateID:   "media-fail-1",
			AggregateType: string(event.AggregateTypeMedia),
			EventType:     string(event.TypeMediaProcessingFailed),
			Data:          makeEventJSON(t, failedData),
			Version:       2,
			CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		}
		if err := p.processEvent(ctx, failEv); err != nil {
			t.Fatalf("MediaProcessingFailedの処理に失敗: %v", err)
		}

		// Read Modelを検証する
		model, err := queries.GetMediaByID(ctx, "media-fail-1")
		if err != nil {
			t.Fatalf("GetMediaByIDが失敗: %v", err)
		}

		if model.Status != "failed" {
			t.Errorf("期待するStatus %q, 実際のStatus %q", "failed", model.Status)
		}
		if model.LastEventVersion != 2 {
			t.Errorf("期待するLastEventVersion 2, 実際のLastEventVersion %d", model.LastEventVersion)
		}
	})
}

func TestProcessEvent_MediaDeleted(t *testing.T) {
	t.Parallel()

	t.Run("正常系_MediaDeletedイベントでステータスがdeletedになる", func(t *testing.T) {
		t.Parallel()

		p, queries, _ := setupTestProjector(t)
		ctx := context.Background()

		// 先にMediaUploadedでレコードを作成する
		uploadedData := event.MediaUploadedData{
			UserID:      "user-123",
			Filename:    "to_delete.jpg",
			ContentType: "image/jpeg",
			Size:        2048,
			StoragePath: "/data/media/media-del-1/to_delete.jpg",
		}
		uploadEv := eventStoreResponse{
			ID:            "event-1",
			AggregateID:   "media-del-1",
			AggregateType: string(event.AggregateTypeMedia),
			EventType:     string(event.TypeMediaUploaded),
			Data:          makeEventJSON(t, uploadedData),
			Version:       1,
			CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		}
		if err := p.processEvent(ctx, uploadEv); err != nil {
			t.Fatalf("MediaUploadedの処理に失敗: %v", err)
		}

		// MediaDeletedイベントを処理する
		deletedData := event.MediaDeletedData{
			UserID: "user-123",
		}
		deleteEv := eventStoreResponse{
			ID:            "event-2",
			AggregateID:   "media-del-1",
			AggregateType: string(event.AggregateTypeMedia),
			EventType:     string(event.TypeMediaDeleted),
			Data:          makeEventJSON(t, deletedData),
			Version:       2,
			CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		}
		if err := p.processEvent(ctx, deleteEv); err != nil {
			t.Fatalf("MediaDeletedの処理に失敗: %v", err)
		}

		// Read Modelを検証する
		model, err := queries.GetMediaByID(ctx, "media-del-1")
		if err != nil {
			t.Fatalf("GetMediaByIDが失敗: %v", err)
		}

		if model.Status != "deleted" {
			t.Errorf("期待するStatus %q, 実際のStatus %q", "deleted", model.Status)
		}
		if model.LastEventVersion != 2 {
			t.Errorf("期待するLastEventVersion 2, 実際のLastEventVersion %d", model.LastEventVersion)
		}
	})
}

func TestProcessEvent_MediaUploadCompensated(t *testing.T) {
	t.Parallel()

	t.Run("正常系_MediaUploadCompensatedイベントでステータスがdeletedになる", func(t *testing.T) {
		t.Parallel()

		p, queries, _ := setupTestProjector(t)
		ctx := context.Background()

		// 先にMediaUploadedでレコードを作成する
		uploadedData := event.MediaUploadedData{
			UserID:      "user-123",
			Filename:    "compensated.jpg",
			ContentType: "image/jpeg",
			Size:        3072,
			StoragePath: "/data/media/media-comp-1/compensated.jpg",
		}
		uploadEv := eventStoreResponse{
			ID:            "event-1",
			AggregateID:   "media-comp-1",
			AggregateType: string(event.AggregateTypeMedia),
			EventType:     string(event.TypeMediaUploaded),
			Data:          makeEventJSON(t, uploadedData),
			Version:       1,
			CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		}
		if err := p.processEvent(ctx, uploadEv); err != nil {
			t.Fatalf("MediaUploadedの処理に失敗: %v", err)
		}

		// MediaUploadCompensatedイベントを処理する
		compensatedData := event.MediaUploadCompensatedData{
			Reason: "Sagaのロールバックにより補償",
			SagaID: "saga-789",
		}
		compensateEv := eventStoreResponse{
			ID:            "event-2",
			AggregateID:   "media-comp-1",
			AggregateType: string(event.AggregateTypeMedia),
			EventType:     string(event.TypeMediaUploadCompensated),
			Data:          makeEventJSON(t, compensatedData),
			Version:       2,
			CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		}
		if err := p.processEvent(ctx, compensateEv); err != nil {
			t.Fatalf("MediaUploadCompensatedの処理に失敗: %v", err)
		}

		// Read Modelを検証する
		model, err := queries.GetMediaByID(ctx, "media-comp-1")
		if err != nil {
			t.Fatalf("GetMediaByIDが失敗: %v", err)
		}

		if model.Status != "deleted" {
			t.Errorf("期待するStatus %q, 実際のStatus %q", "deleted", model.Status)
		}
		if model.LastEventVersion != 2 {
			t.Errorf("期待するLastEventVersion 2, 実際のLastEventVersion %d", model.LastEventVersion)
		}
	})
}

func TestProcessEvent_UnknownEventType(t *testing.T) {
	t.Parallel()

	t.Run("正常系_未知のイベントタイプは無視される", func(t *testing.T) {
		t.Parallel()

		p, _, _ := setupTestProjector(t)
		ctx := context.Background()

		ev := eventStoreResponse{
			ID:            "event-unknown",
			AggregateID:   "media-unknown-1",
			AggregateType: string(event.AggregateTypeMedia),
			EventType:     "UnknownEventType",
			Data:          `{}`,
			Version:       1,
			CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		}

		// エラーなく処理されることを確認する
		if err := p.processEvent(ctx, ev); err != nil {
			t.Errorf("未知のイベントタイプでエラーが発生: %v", err)
		}
	})
}

func TestProcessEvent_NonMediaAggregate(t *testing.T) {
	t.Parallel()

	t.Run("正常系_メディア以外のAggregateTypeは無視される", func(t *testing.T) {
		t.Parallel()

		p, _, _ := setupTestProjector(t)
		ctx := context.Background()

		ev := eventStoreResponse{
			ID:            "event-album",
			AggregateID:   "album-1",
			AggregateType: string(event.AggregateTypeAlbum),
			EventType:     string(event.TypeAlbumCreated),
			Data:          `{"user_id":"user-123","name":"Test Album","description":"desc"}`,
			Version:       1,
			CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		}

		if err := p.processEvent(ctx, ev); err != nil {
			t.Errorf("メディア以外のAggregateTypeでエラーが発生: %v", err)
		}
	})
}

func TestProcessEvent_InvalidJSON(t *testing.T) {
	t.Parallel()

	t.Run("異常系_不正なJSONデータの場合エラーを返す", func(t *testing.T) {
		t.Parallel()

		p, _, _ := setupTestProjector(t)
		ctx := context.Background()

		ev := eventStoreResponse{
			ID:            "event-invalid",
			AggregateID:   "media-invalid-1",
			AggregateType: string(event.AggregateTypeMedia),
			EventType:     string(event.TypeMediaUploaded),
			Data:          `{invalid json}`,
			Version:       1,
			CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		}

		err := p.processEvent(ctx, ev)
		if err == nil {
			t.Error("不正なJSONデータでエラーが返されるべきです")
		}
	})
}

func TestProcessEvent_FullLifecycle(t *testing.T) {
	t.Parallel()

	t.Run("正常系_メディアのアップロードから削除までのライフサイクルを検証する", func(t *testing.T) {
		t.Parallel()

		p, queries, _ := setupTestProjector(t)
		ctx := context.Background()
		aggregateID := "media-lifecycle-1"
		baseTime := time.Now().UTC()

		// ステップ1: アップロード
		uploadedData := event.MediaUploadedData{
			UserID:      "user-lifecycle",
			Filename:    "lifecycle.jpg",
			ContentType: "image/jpeg",
			Size:        5120,
			StoragePath: "/data/media/media-lifecycle-1/lifecycle.jpg",
		}
		uploadEv := eventStoreResponse{
			ID:            "lifecycle-ev-1",
			AggregateID:   aggregateID,
			AggregateType: string(event.AggregateTypeMedia),
			EventType:     string(event.TypeMediaUploaded),
			Data:          makeEventJSON(t, uploadedData),
			Version:       1,
			CreatedAt:     baseTime.Format(time.RFC3339),
		}
		if err := p.processEvent(ctx, uploadEv); err != nil {
			t.Fatalf("ステップ1(Upload)で失敗: %v", err)
		}

		model, _ := queries.GetMediaByID(ctx, aggregateID)
		if model.Status != "uploaded" {
			t.Errorf("ステップ1: 期待するStatus %q, 実際のStatus %q", "uploaded", model.Status)
		}

		// ステップ2: 処理完了
		processedData := event.MediaProcessedData{
			ThumbnailPath: "/data/media/media-lifecycle-1/thumbnail.jpg",
			Width:         640,
			Height:        480,
		}
		processEv := eventStoreResponse{
			ID:            "lifecycle-ev-2",
			AggregateID:   aggregateID,
			AggregateType: string(event.AggregateTypeMedia),
			EventType:     string(event.TypeMediaProcessed),
			Data:          makeEventJSON(t, processedData),
			Version:       2,
			CreatedAt:     baseTime.Add(1 * time.Second).Format(time.RFC3339),
		}
		if err := p.processEvent(ctx, processEv); err != nil {
			t.Fatalf("ステップ2(Process)で失敗: %v", err)
		}

		model, _ = queries.GetMediaByID(ctx, aggregateID)
		if model.Status != "processed" {
			t.Errorf("ステップ2: 期待するStatus %q, 実際のStatus %q", "processed", model.Status)
		}
		if !model.ThumbnailPath.Valid {
			t.Error("ステップ2: ThumbnailPathが設定されていません")
		}

		// ステップ3: 削除
		deletedData := event.MediaDeletedData{
			UserID: "user-lifecycle",
		}
		deleteEv := eventStoreResponse{
			ID:            "lifecycle-ev-3",
			AggregateID:   aggregateID,
			AggregateType: string(event.AggregateTypeMedia),
			EventType:     string(event.TypeMediaDeleted),
			Data:          makeEventJSON(t, deletedData),
			Version:       3,
			CreatedAt:     baseTime.Add(2 * time.Second).Format(time.RFC3339),
		}
		if err := p.processEvent(ctx, deleteEv); err != nil {
			t.Fatalf("ステップ3(Delete)で失敗: %v", err)
		}

		model, _ = queries.GetMediaByID(ctx, aggregateID)
		if model.Status != "deleted" {
			t.Errorf("ステップ3: 期待するStatus %q, 実際のStatus %q", "deleted", model.Status)
		}
		if model.LastEventVersion != 3 {
			t.Errorf("ステップ3: 期待するLastEventVersion 3, 実際のLastEventVersion %d", model.LastEventVersion)
		}
	})
}

func TestNewProjector(t *testing.T) {
	t.Parallel()

	t.Run("正常系_Projectorが正しく初期化される", func(t *testing.T) {
		t.Parallel()

		sqlDB, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatalf("インメモリSQLiteの接続に失敗: %v", err)
		}
		defer sqlDB.Close()

		queries := mediadb.New(sqlDB)
		p := NewProjector(queries, "http://localhost:8084")

		if p.queries != queries {
			t.Error("queriesが正しく設定されていません")
		}
		if p.client == nil {
			t.Error("clientがnilです")
		}
		if p.interval != 2*time.Second {
			t.Errorf("期待するinterval %v, 実際のinterval %v", 2*time.Second, p.interval)
		}
		if !p.lastTimestamp.IsZero() {
			t.Error("lastTimestampはゼロ値であるべきです")
		}
	})
}

func TestProjectorStartStop(t *testing.T) {
	t.Parallel()

	t.Run("正常系_ProjectorのStartとStopが正常に動作する", func(t *testing.T) {
		t.Parallel()

		sqlDB, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatalf("インメモリSQLiteの接続に失敗: %v", err)
		}
		defer sqlDB.Close()

		queries := mediadb.New(sqlDB)
		p := NewProjector(queries, "http://localhost:9999")

		ctx := context.Background()
		p.Start(ctx)

		// cancel関数が設定されていることを確認する
		if p.cancel == nil {
			t.Error("Start後にcancelがnilです")
		}

		// Stopが正常に呼び出せることを確認する
		p.Stop()
	})

	t.Run("正常系_Stopはcancelがnilでも安全に呼び出せる", func(t *testing.T) {
		t.Parallel()

		sqlDB, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatalf("インメモリSQLiteの接続に失敗: %v", err)
		}
		defer sqlDB.Close()

		queries := mediadb.New(sqlDB)
		p := NewProjector(queries, "http://localhost:9999")

		// Start前にStopを呼んでもパニックしないことを確認する
		p.Stop()
	})
}
