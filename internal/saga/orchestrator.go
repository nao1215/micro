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

const (
	// maxRetries はステップ実行の最大リトライ回数。
	maxRetries = 3
	// stuckSagaThreshold はSagaがスタックしたとみなす閾値。
	stuckSagaThreshold = 5 * time.Minute
	// stuckSagaCheckInterval はスタックSagaのチェック間隔。
	stuckSagaCheckInterval = 1 * time.Minute
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

	// 永続化されたオフセットを読み込む
	o.loadOffset()

	// スタックSaga検出をバックグラウンドで開始
	go o.startStuckSagaDetector()

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		o.poll()
	}
}

// loadOffset は永続化されたオフセットを読み込み、lastPolledAtに設定する。
func (o *Orchestrator) loadOffset() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	offset, err := o.queries.GetProjectorOffset(ctx)
	if err != nil {
		log.Println("[Saga] 永続化オフセットなし（初回起動）、1時間前からポーリングします")
		return
	}
	o.lastPolledAt = offset
	log.Printf("[Saga] 永続化オフセットを復元しました: %s", offset.Format(time.RFC3339))
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

			// オフセットを永続化する
			if err := o.queries.UpsertProjectorOffset(ctx, t); err != nil {
				log.Printf("[Saga] オフセット永続化エラー: %v", err)
			}
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

// executeStep はSagaのステップをリトライ付きで実行し、結果をDBに記録する。
// 最大maxRetries回まで指数バックオフでリトライする。
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

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		// 2回目以降は指数バックオフで待機
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			log.Printf("[Saga] ステップ %s リトライ %d/%d（%v後）: saga_id=%s", stepName, attempt, maxRetries, backoff, sagaID)
			time.Sleep(backoff)
		}

		lastErr = action()
		if lastErr == nil {
			// 成功
			_ = o.queries.UpdateSagaStepStatus(ctx, sagadb.UpdateSagaStepStatusParams{
				Status: "completed",
				Result: "{}",
				ID:     stepID,
			})
			if attempt > 0 {
				// リトライ回数を記録
				_ = o.queries.UpdateSagaStepRetry(ctx, sagadb.UpdateSagaStepRetryParams{
					RetryCount: int64(attempt),
					LastError:  "",
					Status:     "completed",
					ID:         stepID,
				})
			}
			return
		}

		// リトライ情報をDBに記録
		_ = o.queries.UpdateSagaStepRetry(ctx, sagadb.UpdateSagaStepRetryParams{
			RetryCount: int64(attempt + 1),
			LastError:  lastErr.Error(),
			Status:     "executing",
			ID:         stepID,
		})
	}

	// 全リトライ失敗
	log.Printf("[Saga] ステップ実行失敗（リトライ上限到達）: step=%s, error=%v, saga_id=%s", stepName, lastErr, sagaID)
	resultJSON, _ := json.Marshal(map[string]string{"error": lastErr.Error()})
	_ = o.queries.UpdateSagaStepStatus(ctx, sagadb.UpdateSagaStepStatusParams{
		Status: "failed",
		Result: string(resultJSON),
		ID:     stepID,
	})
}

// startStuckSagaDetector はスタックしたSagaを定期的に検出して処理するバックグラウンドループ。
func (o *Orchestrator) startStuckSagaDetector() {
	log.Printf("[Saga] スタックSaga検出を開始します（チェック間隔: %v、閾値: %v）", stuckSagaCheckInterval, stuckSagaThreshold)

	ticker := time.NewTicker(stuckSagaCheckInterval)
	defer ticker.Stop()

	for range ticker.C {
		o.checkStuckSagas()
	}
}

// checkStuckSagas はスタックしたSagaを検出し、適切な処理を行う。
func (o *Orchestrator) checkStuckSagas() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	threshold := time.Now().UTC().Add(-stuckSagaThreshold)
	stuckSagas, err := o.queries.ListStuckSagas(ctx, threshold)
	if err != nil {
		log.Printf("[Saga] スタックSaga検索エラー: %v", err)
		return
	}

	for _, saga := range stuckSagas {
		log.Printf("[Saga] スタックSaga検出: saga_id=%s, status=%s, current_step=%s, updated_at=%s",
			saga.ID, saga.Status, saga.CurrentStep, saga.UpdatedAt.Format(time.RFC3339))

		switch saga.Status {
		case "compensating":
			// 補償中のSagaは再補償を試行
			log.Printf("[Saga] 補償中のスタックSagaを再補償します: saga_id=%s", saga.ID)
			var payloadMap map[string]string
			if err := json.Unmarshal([]byte(saga.Payload), &payloadMap); err != nil {
				log.Printf("[Saga] ペイロード解析エラー: saga_id=%s, error=%v", saga.ID, err)
				continue
			}
			aggregateID := payloadMap["media_aggregate_id"]
			if aggregateID != "" {
				o.executeStep(ctx, saga.ID, "compensate_upload_retry", func() error {
					mediaID := extractMediaID(aggregateID)
					compensateReq := map[string]string{
						"saga_id": saga.ID,
						"reason":  "スタック検出による再補償",
					}
					return o.mediaCommandClient.PostJSON(ctx, fmt.Sprintf("/api/v1/media/%s/compensate", mediaID), compensateReq, nil)
				})
			}
			// 再補償後に失敗としてマーク
			if err := o.queries.FailSaga(ctx, saga.ID); err != nil {
				log.Printf("[Saga] Saga失敗記録エラー: %v", err)
			}
		case "in_progress":
			// 進行中のスタックSagaは失敗としてマーク
			log.Printf("[Saga] 進行中のスタックSagaを失敗としてマークします: saga_id=%s", saga.ID)
			if err := o.queries.FailSaga(ctx, saga.ID); err != nil {
				log.Printf("[Saga] Saga失敗記録エラー: %v", err)
			}
		}
	}
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
