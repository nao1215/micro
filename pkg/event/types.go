package event

import (
	"encoding/json"
	"time"
)

// AggregateType はイベントの対象となるエンティティの種類を表す。
type AggregateType string

const (
	// AggregateTypeMedia はメディアエンティティを表す。
	AggregateTypeMedia AggregateType = "Media"
	// AggregateTypeAlbum はアルバムエンティティを表す。
	AggregateTypeAlbum AggregateType = "Album"
	// AggregateTypeUser はユーザーエンティティを表す。
	AggregateTypeUser AggregateType = "User"
)

// Type はイベントの種類を表す。
type Type string

const (
	// TypeMediaUploaded はメディアファイルがアップロードされたことを表す。
	TypeMediaUploaded Type = "MediaUploaded"
	// TypeMediaProcessed はサムネイル生成等のメディア処理が完了したことを表す。
	TypeMediaProcessed Type = "MediaProcessed"
	// TypeMediaProcessingFailed はメディア処理が失敗したことを表す。
	TypeMediaProcessingFailed Type = "MediaProcessingFailed"
	// TypeMediaDeleted はメディアが削除されたことを表す。
	TypeMediaDeleted Type = "MediaDeleted"
	// TypeMediaUploadCompensated はメディアアップロードの補償アクションが実行されたことを表す。
	TypeMediaUploadCompensated Type = "MediaUploadCompensated"

	// TypeAlbumCreated はアルバムが作成されたことを表す。
	TypeAlbumCreated Type = "AlbumCreated"
	// TypeAlbumDeleted はアルバムが削除されたことを表す。
	TypeAlbumDeleted Type = "AlbumDeleted"
	// TypeMediaAddedToAlbum はメディアがアルバムに追加されたことを表す。
	TypeMediaAddedToAlbum Type = "MediaAddedToAlbum"
	// TypeMediaRemovedFromAlbum はメディアがアルバムから削除されたことを表す。
	TypeMediaRemovedFromAlbum Type = "MediaRemovedFromAlbum"

	// TypeNotificationSent は通知が送信されたことを表す。
	TypeNotificationSent Type = "NotificationSent"
)

// Event はEvent Sourcingにおける不変のイベントレコードを表す。
// すべての状態変更はこの構造体としてEvent Storeに永続化される。
type Event struct {
	// ID はイベントの一意識別子（UUID）。
	ID string `json:"id"`
	// AggregateID は対象エンティティの識別子。
	AggregateID string `json:"aggregate_id"`
	// AggregateType は対象エンティティの種類。
	AggregateType AggregateType `json:"aggregate_type"`
	// EventType はイベントの種類。
	EventType Type `json:"event_type"`
	// Data はイベント固有のデータ（JSON形式）。
	Data json.RawMessage `json:"data"`
	// Version はAggregate内でのイベントの順序番号。楽観的排他制御に使用する。
	Version int64 `json:"version"`
	// CreatedAt はイベントが作成された日時。
	CreatedAt time.Time `json:"created_at"`
}

// MediaUploadedData はMediaUploadedイベントのデータ。
type MediaUploadedData struct {
	// UserID はアップロードしたユーザーのID。
	UserID string `json:"user_id"`
	// Filename は元のファイル名。
	Filename string `json:"filename"`
	// ContentType はファイルのMIMEタイプ。
	ContentType string `json:"content_type"`
	// Size はファイルサイズ（バイト）。
	Size int64 `json:"size"`
	// StoragePath はファイルの保存パス。
	StoragePath string `json:"storage_path"`
}

// MediaProcessedData はMediaProcessedイベントのデータ。
type MediaProcessedData struct {
	// ThumbnailPath はサムネイル画像の保存パス。
	ThumbnailPath string `json:"thumbnail_path"`
	// Width は画像/動画の幅（ピクセル）。
	Width int `json:"width"`
	// Height は画像/動画の高さ（ピクセル）。
	Height int `json:"height"`
	// DurationSeconds は動画の長さ（秒）。画像の場合は0。
	DurationSeconds float64 `json:"duration_seconds,omitempty"`
}

// MediaProcessingFailedData はMediaProcessingFailedイベントのデータ。
type MediaProcessingFailedData struct {
	// Reason は処理失敗の理由。
	Reason string `json:"reason"`
}

// MediaDeletedData はMediaDeletedイベントのデータ。
type MediaDeletedData struct {
	// UserID は削除を実行したユーザーのID。
	UserID string `json:"user_id"`
}

// MediaUploadCompensatedData はMediaUploadCompensatedイベントのデータ。
type MediaUploadCompensatedData struct {
	// Reason は補償アクションが実行された理由。
	Reason string `json:"reason"`
	// SagaID は関連するSagaのID。
	SagaID string `json:"saga_id"`
}

// AlbumCreatedData はAlbumCreatedイベントのデータ。
type AlbumCreatedData struct {
	// UserID はアルバムを作成したユーザーのID。
	UserID string `json:"user_id"`
	// Name はアルバム名。
	Name string `json:"name"`
	// Description はアルバムの説明。
	Description string `json:"description"`
}

// AlbumDeletedData はAlbumDeletedイベントのデータ。
type AlbumDeletedData struct {
	// UserID はアルバムを削除したユーザーのID。
	UserID string `json:"user_id"`
}

// MediaAddedToAlbumData はMediaAddedToAlbumイベントのデータ。
type MediaAddedToAlbumData struct {
	// MediaID は追加されたメディアのID。
	MediaID string `json:"media_id"`
}

// MediaRemovedFromAlbumData はMediaRemovedFromAlbumイベントのデータ。
type MediaRemovedFromAlbumData struct {
	// MediaID は削除されたメディアのID。
	MediaID string `json:"media_id"`
}

// NotificationSentData はNotificationSentイベントのデータ。
type NotificationSentData struct {
	// UserID は通知先のユーザーID。
	UserID string `json:"user_id"`
	// Title は通知のタイトル。
	Title string `json:"title"`
	// Message は通知メッセージ。
	Message string `json:"message"`
}
