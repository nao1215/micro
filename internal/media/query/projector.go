package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/nao1215/micro/pkg/event"
	"github.com/nao1215/micro/pkg/httpclient"

	mediadb "github.com/nao1215/micro/internal/media/query/db"
)

// Projector はEvent Storeのイベントをポーリングし、Read Modelを更新するバックグラウンドプロセス。
// Event Sourcingにおける投影（Projection）を担当する。
type Projector struct {
	// queries はsqlcが生成したクエリ実行オブジェクト。
	queries *mediadb.Queries
	// client はEvent Storeとの通信用HTTPクライアント。
	client *httpclient.Client
	// interval はポーリング間隔。
	interval time.Duration
	// lastTimestamp は最後にポーリングしたイベントのタイムスタンプ。
	lastTimestamp time.Time
	// mu はlastTimestampへの並行アクセスを保護するミューテックス。
	mu sync.Mutex
	// cancel はバックグラウンドゴルーチンを停止するためのキャンセル関数。
	cancel context.CancelFunc
}

// NewProjector は新しいProjectorを生成する。
// eventstoreURL はEvent StoreのベースURL（例: "http://localhost:8084"）。
func NewProjector(queries *mediadb.Queries, eventstoreURL string) *Projector {
	return &Projector{
		queries:       queries,
		client:        httpclient.New(eventstoreURL),
		interval:      2 * time.Second,
		lastTimestamp: time.Time{},
	}
}

// Start はバックグラウンドでEvent Storeのポーリングを開始する。
// 定期的にEvent Storeから新しいイベントを取得してRead Modelに反映する。
func (p *Projector) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	p.cancel = cancel

	go func() {
		log.Println("Projector: Event Storeポーリングを開始します")
		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Println("Projector: ポーリングを停止しました")
				return
			case <-ticker.C:
				if err := p.poll(ctx); err != nil {
					log.Printf("Projector: ポーリングエラー: %v", err)
				}
			}
		}
	}()
}

// Stop はバックグラウンドのポーリングを停止する。
func (p *Projector) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
}

// eventStoreResponse はEvent Store APIから返されるイベントのJSON構造。
type eventStoreResponse struct {
	// ID はイベントの一意識別子。
	ID string `json:"id"`
	// AggregateID は対象エンティティの識別子。
	AggregateID string `json:"aggregate_id"`
	// AggregateType は対象エンティティの種類。
	AggregateType string `json:"aggregate_type"`
	// EventType はイベントの種類。
	EventType string `json:"event_type"`
	// Data はイベント固有のデータ（JSON文字列）。
	Data string `json:"data"`
	// Version はAggregate内でのイベントの順序番号。
	Version int64 `json:"version"`
	// CreatedAt はイベントが作成された日時（RFC3339形式）。
	CreatedAt string `json:"created_at"`
}

// poll はEvent Storeから新しいイベントを取得してRead Modelに反映する。
func (p *Projector) poll(ctx context.Context) error {
	p.mu.Lock()
	since := p.lastTimestamp
	p.mu.Unlock()

	sinceStr := since.UTC().Format(time.RFC3339)
	path := fmt.Sprintf("/api/v1/events/since?since=%s", url.QueryEscape(sinceStr))

	var events []eventStoreResponse
	if err := p.client.GetJSON(ctx, path, &events); err != nil {
		return fmt.Errorf("Event Storeからのイベント取得に失敗: %w", err)
	}

	if len(events) == 0 {
		return nil
	}

	var latestTimestamp time.Time
	for _, ev := range events {
		if err := p.processEvent(ctx, ev); err != nil {
			log.Printf("Projector: イベント処理エラー (id=%s, type=%s): %v", ev.ID, ev.EventType, err)
			continue
		}

		createdAt, err := time.Parse(time.RFC3339, ev.CreatedAt)
		if err == nil && createdAt.After(latestTimestamp) {
			latestTimestamp = createdAt
		}
	}

	if !latestTimestamp.IsZero() {
		p.mu.Lock()
		// 同じイベントを再取得しないように1ナノ秒進める
		p.lastTimestamp = latestTimestamp.Add(1 * time.Nanosecond)
		p.mu.Unlock()
	}

	log.Printf("Projector: %d件のイベントを処理しました", len(events))
	return nil
}

// processEvent は1つのイベントをRead Modelに反映する。
// イベントタイプに応じて適切なRead Model更新処理を呼び出す。
func (p *Projector) processEvent(ctx context.Context, ev eventStoreResponse) error {
	// メディア関連のイベントのみ処理する
	if ev.AggregateType != string(event.AggregateTypeMedia) {
		return nil
	}

	switch event.Type(ev.EventType) {
	case event.TypeMediaUploaded:
		return p.handleMediaUploaded(ctx, ev)
	case event.TypeMediaProcessed:
		return p.handleMediaProcessed(ctx, ev)
	case event.TypeMediaProcessingFailed:
		return p.handleMediaProcessingFailed(ctx, ev)
	case event.TypeMediaDeleted:
		return p.handleMediaDeleted(ctx, ev)
	case event.TypeMediaUploadCompensated:
		return p.handleMediaUploadCompensated(ctx, ev)
	default:
		return nil
	}
}

// handleMediaUploaded はMediaUploadedイベントをRead Modelに反映する。
// 新しいメディアレコードをstatus=uploadedで挿入する。
func (p *Projector) handleMediaUploaded(ctx context.Context, ev eventStoreResponse) error {
	var data event.MediaUploadedData
	if err := json.Unmarshal([]byte(ev.Data), &data); err != nil {
		return fmt.Errorf("MediaUploadedDataのデシリアライズに失敗: %w", err)
	}

	createdAt, err := time.Parse(time.RFC3339, ev.CreatedAt)
	if err != nil {
		createdAt = time.Now().UTC()
	}

	return p.queries.UpsertMediaReadModel(ctx, mediadb.UpsertMediaReadModelParams{
		ID:               ev.AggregateID,
		UserID:           data.UserID,
		Filename:         data.Filename,
		ContentType:      data.ContentType,
		Size:             data.Size,
		StoragePath:      data.StoragePath,
		Status:           "uploaded",
		LastEventVersion: ev.Version,
		UploadedAt:       createdAt,
	})
}

// handleMediaProcessed はMediaProcessedイベントをRead Modelに反映する。
// サムネイルパス、幅、高さを更新し、status=processedに変更する。
func (p *Projector) handleMediaProcessed(ctx context.Context, ev eventStoreResponse) error {
	var data event.MediaProcessedData
	if err := json.Unmarshal([]byte(ev.Data), &data); err != nil {
		return fmt.Errorf("MediaProcessedDataのデシリアライズに失敗: %w", err)
	}

	return p.queries.UpdateMediaProcessed(ctx, mediadb.UpdateMediaProcessedParams{
		ThumbnailPath: sql.NullString{
			String: data.ThumbnailPath,
			Valid:  data.ThumbnailPath != "",
		},
		Width: sql.NullInt64{
			Int64: int64(data.Width),
			Valid: data.Width != 0,
		},
		Height: sql.NullInt64{
			Int64: int64(data.Height),
			Valid: data.Height != 0,
		},
		DurationSeconds: sql.NullFloat64{
			Float64: data.DurationSeconds,
			Valid:   data.DurationSeconds != 0,
		},
		LastEventVersion: ev.Version,
		ID:               ev.AggregateID,
	})
}

// handleMediaProcessingFailed はMediaProcessingFailedイベントをRead Modelに反映する。
// status=failedに変更する。
func (p *Projector) handleMediaProcessingFailed(ctx context.Context, ev eventStoreResponse) error {
	return p.queries.UpdateMediaStatus(ctx, mediadb.UpdateMediaStatusParams{
		Status:           "failed",
		LastEventVersion: ev.Version,
		ID:               ev.AggregateID,
	})
}

// handleMediaDeleted はMediaDeletedイベントをRead Modelに反映する。
// status=deletedに変更する。
func (p *Projector) handleMediaDeleted(ctx context.Context, ev eventStoreResponse) error {
	return p.queries.UpdateMediaStatus(ctx, mediadb.UpdateMediaStatusParams{
		Status:           "deleted",
		LastEventVersion: ev.Version,
		ID:               ev.AggregateID,
	})
}

// handleMediaUploadCompensated はMediaUploadCompensatedイベントをRead Modelに反映する。
// 補償アクションとしてstatus=deletedに変更する。
func (p *Projector) handleMediaUploadCompensated(ctx context.Context, ev eventStoreResponse) error {
	return p.queries.UpdateMediaStatus(ctx, mediadb.UpdateMediaStatusParams{
		Status:           "deleted",
		LastEventVersion: ev.Version,
		ID:               ev.AggregateID,
	})
}

// RebuildFromEventStore はRead Modelを全削除し、Event Storeの全イベントから再構築する。
// Read Modelが破損した場合や整合性を回復する必要がある場合に使用する。
func (p *Projector) RebuildFromEventStore(ctx context.Context) error {
	log.Println("Projector: Read Modelの再構築を開始します")

	// Read Modelの全データを削除
	if err := p.queries.DeleteAllMediaReadModels(ctx); err != nil {
		return fmt.Errorf("Read Modelの全削除に失敗: %w", err)
	}

	// Event Storeから全イベントを取得
	var events []eventStoreResponse
	if err := p.client.GetJSON(ctx, "/api/v1/events", &events); err != nil {
		return fmt.Errorf("Event Storeからの全イベント取得に失敗: %w", err)
	}

	// 全イベントを順次処理してRead Modelを再構築
	var processedCount int
	for _, ev := range events {
		if err := p.processEvent(ctx, ev); err != nil {
			log.Printf("Projector: 再構築中のイベント処理エラー (id=%s, type=%s): %v", ev.ID, ev.EventType, err)
			continue
		}
		processedCount++
	}

	// lastTimestampをリセットして最新のイベント以降からポーリングを再開する
	if len(events) > 0 {
		lastEvent := events[len(events)-1]
		if createdAt, err := time.Parse(time.RFC3339, lastEvent.CreatedAt); err == nil {
			p.mu.Lock()
			p.lastTimestamp = createdAt.Add(1 * time.Nanosecond)
			p.mu.Unlock()
		}
	}

	log.Printf("Projector: Read Modelの再構築が完了しました（%d件のイベントを処理）", processedCount)
	return nil
}
