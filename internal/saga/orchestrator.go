package saga

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	sagadb "github.com/nao1215/micro/internal/saga/db"
	"github.com/nao1215/micro/pkg/event"
	"github.com/nao1215/micro/pkg/httpclient"
)

// Orchestrator はSagaの実行を管理するオーケストレータ。
// Event Storeをポーリングしてイベントを受信し、対応するSagaを進行させる。
// 失敗時には逆順に補償アクションを実行する。
type Orchestrator struct {
	// queries はSaga状態管理用のDBクエリ。
	queries *sagadb.Queries
	// eventStoreClient はEvent StoreへのHTTPクライアント。
	eventStoreClient *httpclient.Client
	// mediaCommandClient はmedia-commandサービスへのHTTPクライアント。
	mediaCommandClient *httpclient.Client
	// albumClient はalbumサービスへのHTTPクライアント。
	albumClient *httpclient.Client
	// notificationClient はnotificationサービスへのHTTPクライアント。
	notificationClient *httpclient.Client
	// lastPolledAt は最後にEvent Storeをポーリングした日時。
	lastPolledAt time.Time
}

// NewOrchestrator は新しいSagaオーケストレータを生成する。
func NewOrchestrator(
	queries *sagadb.Queries,
	eventStoreClient *httpclient.Client,
	mediaCommandClient *httpclient.Client,
	albumClient *httpclient.Client,
	notificationClient *httpclient.Client,
) *Orchestrator {
	return &Orchestrator{
		queries:            queries,
		eventStoreClient:   eventStoreClient,
		mediaCommandClient: mediaCommandClient,
		albumClient:        albumClient,
		notificationClient: notificationClient,
		lastPolledAt:       time.Now().UTC().Add(-1 * time.Hour),
	}
}

// eventStoreEvent はEvent StoreのAPIレスポンスに対応する構造体。
type eventStoreEvent struct {
	ID            string `json:"id"`
	AggregateID   string `json:"aggregate_id"`
	AggregateType string `json:"aggregate_type"`
	EventType     string `json:"event_type"`
	Data          string `json:"data"`
	Version       int64  `json:"version"`
	CreatedAt     string `json:"created_at"`
}

// Start はイベントポーリングループを開始する。
// バックグラウンドgoroutineとして呼び出されることを想定している。
func (o *Orchestrator) Start() {
	log.Println("[Saga] オーケストレータを開始します。イベントポーリング間隔: 3秒")

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		o.poll()
	}
}

// poll はEvent Storeから新しいイベントを取得し、Sagaを進行させる。
func (o *Orchestrator) poll() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sinceParam := o.lastPolledAt.Format(time.RFC3339)
	path := fmt.Sprintf("/api/v1/events/since?since=%s", sinceParam)

	var events []eventStoreEvent
	if err := o.eventStoreClient.GetJSON(ctx, path, &events); err != nil {
		log.Printf("[Saga] イベントポーリングエラー: %v", err)
		return
	}

	for i := range events {
		o.HandleEvent(ctx, events[i].EventType, events[i].AggregateID, events[i].Data)
	}

	if len(events) > 0 {
		// 最後のイベントの作成日時を記録して、次回ポーリングの起点にする
		lastEvent := events[len(events)-1]
		if t, err := time.Parse(time.RFC3339, lastEvent.CreatedAt); err == nil {
			o.lastPolledAt = t
		}
	}
}

// HandleEvent はイベントを受信し、対応するSagaアクションを実行する。
// ポーリングと手動通知の両方から呼び出される。
func (o *Orchestrator) HandleEvent(ctx context.Context, eventType, aggregateID, data string) {
	switch event.Type(eventType) {
	case event.TypeMediaUploaded:
		o.startMediaUploadSaga(ctx, aggregateID, data)
	case event.TypeMediaProcessed:
		o.advanceSagaOnProcessed(ctx, aggregateID)
	case event.TypeMediaProcessingFailed:
		o.compensateOnProcessingFailed(ctx, aggregateID, data)
	case event.TypeMediaAddedToAlbum:
		o.advanceSagaOnAlbumAdded(ctx, aggregateID)
	}
}

// startMediaUploadSaga はメディアアップロードSagaを新規開始する。
// Step1: Sagaレコード作成 → Step2: サムネイル生成依頼
func (o *Orchestrator) startMediaUploadSaga(ctx context.Context, aggregateID, data string) {
	sagaID := uuid.New().String()

	// Sagaの初期ペイロードにメディアIDとアップロードデータを保存
	payload, _ := json.Marshal(map[string]string{
		"media_aggregate_id": aggregateID,
		"upload_data":        data,
	})

	if err := o.queries.CreateSaga(ctx, sagadb.CreateSagaParams{
		ID:          sagaID,
		SagaType:    "media_upload",
		CurrentStep: "process_media",
		Payload:     string(payload),
	}); err != nil {
		log.Printf("[Saga] Saga作成エラー: %v", err)
		return
	}

	log.Printf("[Saga] メディアアップロードSaga開始: saga_id=%s, aggregate_id=%s", sagaID, aggregateID)

	// Step: サムネイル生成を依頼
	o.executeStep(ctx, sagaID, "process_media", func() error {
		// media-commandの /api/v1/media/{id}/process を呼び出す
		mediaID := extractMediaID(aggregateID)
		return o.mediaCommandClient.PostJSON(ctx, fmt.Sprintf("/api/v1/media/%s/process", mediaID), nil, nil)
	})
}

// advanceSagaOnProcessed はMediaProcessedイベント受信時にSagaを進行させる。
// サムネイル生成成功 → デフォルトアルバムに追加依頼
func (o *Orchestrator) advanceSagaOnProcessed(ctx context.Context, aggregateID string) {
	saga := o.findActiveSagaByAggregateID(ctx, aggregateID)
	if saga == nil {
		return
	}

	// Sagaを次のステップに進める
	if err := o.queries.UpdateSagaStep(ctx, sagadb.UpdateSagaStepParams{
		CurrentStep: "add_to_album",
		Status:      "in_progress",
		Payload:     saga.Payload,
		ID:          saga.ID,
	}); err != nil {
		log.Printf("[Saga] Saga更新エラー: %v", err)
		return
	}

	// Step: デフォルトアルバムにメディアを追加
	o.executeStep(ctx, saga.ID, "add_to_album", func() error {
		var payloadMap map[string]string
		if err := json.Unmarshal([]byte(saga.Payload), &payloadMap); err != nil {
			return fmt.Errorf("ペイロードの解析に失敗: %w", err)
		}

		// アルバムサービスにメディア追加を依頼
		// ユーザーIDはアップロードデータから取得
		var uploadData event.MediaUploadedData
		if err := json.Unmarshal([]byte(payloadMap["upload_data"]), &uploadData); err != nil {
			return fmt.Errorf("アップロードデータの解析に失敗: %w", err)
		}

		mediaID := extractMediaID(payloadMap["media_aggregate_id"])
		addReq := map[string]string{
			"media_id": mediaID,
			"user_id":  uploadData.UserID,
		}
		return o.albumClient.PostJSON(ctx, "/api/v1/albums/default/media", addReq, nil)
	})
}

// advanceSagaOnAlbumAdded はMediaAddedToAlbumイベント受信時にSagaを進行させる。
// アルバム追加成功 → 通知送信依頼 → Saga完了
func (o *Orchestrator) advanceSagaOnAlbumAdded(ctx context.Context, aggregateID string) {
	// アルバムのaggregate_idなので、関連するSagaを探す
	activeSagas, err := o.queries.ListActiveSagas(ctx)
	if err != nil {
		log.Printf("[Saga] アクティブSaga取得エラー: %v", err)
		return
	}

	for _, saga := range activeSagas {
		if saga.CurrentStep != "add_to_album" {
			continue
		}

		// Sagaを次のステップに進める
		if err := o.queries.UpdateSagaStep(ctx, sagadb.UpdateSagaStepParams{
			CurrentStep: "send_notification",
			Status:      "in_progress",
			Payload:     saga.Payload,
			ID:          saga.ID,
		}); err != nil {
			log.Printf("[Saga] Saga更新エラー: %v", err)
			continue
		}

		// Step: 完了通知を送信
		o.executeStep(ctx, saga.ID, "send_notification", func() error {
			var payloadMap map[string]string
			if err := json.Unmarshal([]byte(saga.Payload), &payloadMap); err != nil {
				return fmt.Errorf("ペイロードの解析に失敗: %w", err)
			}

			var uploadData event.MediaUploadedData
			if err := json.Unmarshal([]byte(payloadMap["upload_data"]), &uploadData); err != nil {
				return fmt.Errorf("アップロードデータの解析に失敗: %w", err)
			}

			notifReq := map[string]string{
				"user_id": uploadData.UserID,
				"title":   "アップロード完了",
				"message": fmt.Sprintf("メディア「%s」のアップロードと処理が完了しました。", uploadData.Filename),
			}
			return o.notificationClient.PostJSON(ctx, "/api/v1/internal/send", notifReq, nil)
		})

		// Saga完了
		if err := o.queries.CompleteSaga(ctx, saga.ID); err != nil {
			log.Printf("[Saga] Saga完了エラー: %v", err)
		} else {
			log.Printf("[Saga] メディアアップロードSaga完了: saga_id=%s", saga.ID)
		}
	}
}

// compensateOnProcessingFailed はメディア処理失敗時に補償アクションを実行する。
// サムネイル生成失敗 → アップロード済みファイルの無効化（補償）→ Saga失敗
func (o *Orchestrator) compensateOnProcessingFailed(ctx context.Context, aggregateID, data string) {
	saga := o.findActiveSagaByAggregateID(ctx, aggregateID)
	if saga == nil {
		return
	}

	log.Printf("[Saga] 補償アクション開始: saga_id=%s, reason=メディア処理失敗", saga.ID)

	// Sagaを補償中状態に更新
	if err := o.queries.UpdateSagaStep(ctx, sagadb.UpdateSagaStepParams{
		CurrentStep: "compensate_upload",
		Status:      "compensating",
		Payload:     saga.Payload,
		ID:          saga.ID,
	}); err != nil {
		log.Printf("[Saga] Saga更新エラー: %v", err)
	}

	// 補償アクション: アップロード済みメディアの無効化
	o.executeStep(ctx, saga.ID, "compensate_upload", func() error {
		mediaID := extractMediaID(aggregateID)
		compensateReq := map[string]string{
			"saga_id": saga.ID,
			"reason":  "サムネイル生成に失敗したため、アップロードを無効化",
		}
		return o.mediaCommandClient.PostJSON(ctx, fmt.Sprintf("/api/v1/media/%s/compensate", mediaID), compensateReq, nil)
	})

	// Saga失敗として記録
	if err := o.queries.FailSaga(ctx, saga.ID); err != nil {
		log.Printf("[Saga] Saga失敗記録エラー: %v", err)
	} else {
		log.Printf("[Saga] メディアアップロードSaga失敗（補償完了）: saga_id=%s", saga.ID)
	}
}

// executeStep はSagaのステップを実行し、結果をDBに記録する。
func (o *Orchestrator) executeStep(ctx context.Context, sagaID, stepName string, action func() error) {
	stepID := uuid.New().String()

	// ステップ開始を記録
	if err := o.queries.CreateSagaStep(ctx, sagadb.CreateSagaStepParams{
		ID:       stepID,
		SagaID:   sagaID,
		StepName: stepName,
		Status:   "executing",
	}); err != nil {
		log.Printf("[Saga] ステップ記録エラー: %v", err)
	}

	// ステップを実行
	if err := action(); err != nil {
		log.Printf("[Saga] ステップ実行エラー: step=%s, error=%v", stepName, err)
		resultJSON, _ := json.Marshal(map[string]string{"error": err.Error()})
		_ = o.queries.UpdateSagaStepStatus(ctx, sagadb.UpdateSagaStepStatusParams{
			Status: "failed",
			Result: string(resultJSON),
			ID:     stepID,
		})
		return
	}

	// ステップ完了を記録
	_ = o.queries.UpdateSagaStepStatus(ctx, sagadb.UpdateSagaStepStatusParams{
		Status: "completed",
		Result: "{}",
		ID:     stepID,
	})
}

// findActiveSagaByAggregateID はメディアのaggregate_idに対応するアクティブなSagaを検索する。
func (o *Orchestrator) findActiveSagaByAggregateID(ctx context.Context, aggregateID string) *sagadb.Saga {
	sagas, err := o.queries.ListActiveSagas(ctx)
	if err != nil {
		log.Printf("[Saga] アクティブSaga取得エラー: %v", err)
		return nil
	}

	for _, saga := range sagas {
		var payloadMap map[string]string
		if err := json.Unmarshal([]byte(saga.Payload), &payloadMap); err != nil {
			continue
		}
		if payloadMap["media_aggregate_id"] == aggregateID {
			return &saga
		}
	}
	return nil
}

// extractMediaID はaggregate_id（例: "media-xxxx"）からメディアIDを抽出する。
// aggregate_idがそのままIDとして使える場合はそのまま返す。
func extractMediaID(aggregateID string) string {
	return aggregateID
}
