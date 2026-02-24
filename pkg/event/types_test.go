package event

import (
	"encoding/json"
	"testing"
	"time"
)

// TestAggregateTypeConstants はAggregateType定数の値を検証する。
func TestAggregateTypeConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  AggregateType
		want string
	}{
		{
			name: "AggregateTypeMediaの値が正しいこと",
			got:  AggregateTypeMedia,
			want: "Media",
		},
		{
			name: "AggregateTypeAlbumの値が正しいこと",
			got:  AggregateTypeAlbum,
			want: "Album",
		},
		{
			name: "AggregateTypeUserの値が正しいこと",
			got:  AggregateTypeUser,
			want: "User",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if string(tt.got) != tt.want {
				t.Errorf("AggregateType = %q, want %q", tt.got, tt.want)
			}
		})
	}
}

// TestTypeConstants はType定数の値を検証する。
func TestTypeConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  Type
		want string
	}{
		{
			name: "TypeMediaUploadedの値が正しいこと",
			got:  TypeMediaUploaded,
			want: "MediaUploaded",
		},
		{
			name: "TypeMediaProcessedの値が正しいこと",
			got:  TypeMediaProcessed,
			want: "MediaProcessed",
		},
		{
			name: "TypeMediaProcessingFailedの値が正しいこと",
			got:  TypeMediaProcessingFailed,
			want: "MediaProcessingFailed",
		},
		{
			name: "TypeMediaDeletedの値が正しいこと",
			got:  TypeMediaDeleted,
			want: "MediaDeleted",
		},
		{
			name: "TypeMediaUploadCompensatedの値が正しいこと",
			got:  TypeMediaUploadCompensated,
			want: "MediaUploadCompensated",
		},
		{
			name: "TypeAlbumCreatedの値が正しいこと",
			got:  TypeAlbumCreated,
			want: "AlbumCreated",
		},
		{
			name: "TypeAlbumDeletedの値が正しいこと",
			got:  TypeAlbumDeleted,
			want: "AlbumDeleted",
		},
		{
			name: "TypeMediaAddedToAlbumの値が正しいこと",
			got:  TypeMediaAddedToAlbum,
			want: "MediaAddedToAlbum",
		},
		{
			name: "TypeMediaRemovedFromAlbumの値が正しいこと",
			got:  TypeMediaRemovedFromAlbum,
			want: "MediaRemovedFromAlbum",
		},
		{
			name: "TypeNotificationSentの値が正しいこと",
			got:  TypeNotificationSent,
			want: "NotificationSent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if string(tt.got) != tt.want {
				t.Errorf("Type = %q, want %q", tt.got, tt.want)
			}
		})
	}
}

// TestEventJSONSerialization はEvent構造体のJSONシリアライズ/デシリアライズを検証する。
func TestEventJSONSerialization(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	original := Event{
		ID:            "test-id-123",
		AggregateID:   "aggregate-456",
		AggregateType: AggregateTypeMedia,
		EventType:     TypeMediaUploaded,
		Data:          json.RawMessage(`{"user_id":"user-1","filename":"photo.jpg"}`),
		Version:       1,
		CreatedAt:     now,
	}

	t.Run("Event構造体をJSONにシリアライズできること", func(t *testing.T) {
		t.Parallel()

		jsonBytes, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("json.Marshal()でエラーが発生: %v", err)
		}

		var decoded Event
		if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
			t.Fatalf("json.Unmarshal()でエラーが発生: %v", err)
		}

		if decoded.ID != original.ID {
			t.Errorf("ID = %q, want %q", decoded.ID, original.ID)
		}
		if decoded.AggregateID != original.AggregateID {
			t.Errorf("AggregateID = %q, want %q", decoded.AggregateID, original.AggregateID)
		}
		if decoded.AggregateType != original.AggregateType {
			t.Errorf("AggregateType = %q, want %q", decoded.AggregateType, original.AggregateType)
		}
		if decoded.EventType != original.EventType {
			t.Errorf("EventType = %q, want %q", decoded.EventType, original.EventType)
		}
		if decoded.Version != original.Version {
			t.Errorf("Version = %d, want %d", decoded.Version, original.Version)
		}
		if !decoded.CreatedAt.Equal(original.CreatedAt) {
			t.Errorf("CreatedAt = %v, want %v", decoded.CreatedAt, original.CreatedAt)
		}
	})

	t.Run("EventのJSONフィールド名がスネークケースであること", func(t *testing.T) {
		t.Parallel()

		jsonBytes, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("json.Marshal()でエラーが発生: %v", err)
		}

		var raw map[string]json.RawMessage
		if err := json.Unmarshal(jsonBytes, &raw); err != nil {
			t.Fatalf("json.Unmarshal()でエラーが発生: %v", err)
		}

		expectedKeys := []string{"id", "aggregate_id", "aggregate_type", "event_type", "data", "version", "created_at"}
		for _, key := range expectedKeys {
			if _, ok := raw[key]; !ok {
				t.Errorf("JSONに期待するキー %q が存在しない", key)
			}
		}
	})
}

// TestMediaUploadedDataJSON はMediaUploadedDataのJSONシリアライズを検証する。
func TestMediaUploadedDataJSON(t *testing.T) {
	t.Parallel()

	data := MediaUploadedData{
		UserID:      "user-123",
		Filename:    "photo.jpg",
		ContentType: "image/jpeg",
		Size:        1024000,
		StoragePath: "/uploads/user-123/photo.jpg",
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("json.Marshal()でエラーが発生: %v", err)
	}

	var decoded MediaUploadedData
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal()でエラーが発生: %v", err)
	}

	if decoded.UserID != data.UserID {
		t.Errorf("UserID = %q, want %q", decoded.UserID, data.UserID)
	}
	if decoded.Filename != data.Filename {
		t.Errorf("Filename = %q, want %q", decoded.Filename, data.Filename)
	}
	if decoded.ContentType != data.ContentType {
		t.Errorf("ContentType = %q, want %q", decoded.ContentType, data.ContentType)
	}
	if decoded.Size != data.Size {
		t.Errorf("Size = %d, want %d", decoded.Size, data.Size)
	}
	if decoded.StoragePath != data.StoragePath {
		t.Errorf("StoragePath = %q, want %q", decoded.StoragePath, data.StoragePath)
	}
}

// TestMediaProcessedDataJSON はMediaProcessedDataのJSONシリアライズを検証する。
func TestMediaProcessedDataJSON(t *testing.T) {
	t.Parallel()

	t.Run("動画データの場合DurationSecondsが含まれること", func(t *testing.T) {
		t.Parallel()

		data := MediaProcessedData{
			ThumbnailPath:   "/thumbs/media-1.jpg",
			Width:           1920,
			Height:          1080,
			DurationSeconds: 120.5,
		}

		jsonBytes, err := json.Marshal(data)
		if err != nil {
			t.Fatalf("json.Marshal()でエラーが発生: %v", err)
		}

		var decoded MediaProcessedData
		if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
			t.Fatalf("json.Unmarshal()でエラーが発生: %v", err)
		}

		if decoded.DurationSeconds != 120.5 {
			t.Errorf("DurationSeconds = %f, want %f", decoded.DurationSeconds, 120.5)
		}
	})

	t.Run("画像データの場合DurationSecondsがomitされること", func(t *testing.T) {
		t.Parallel()

		data := MediaProcessedData{
			ThumbnailPath:   "/thumbs/media-2.jpg",
			Width:           800,
			Height:          600,
			DurationSeconds: 0,
		}

		jsonBytes, err := json.Marshal(data)
		if err != nil {
			t.Fatalf("json.Marshal()でエラーが発生: %v", err)
		}

		var raw map[string]json.RawMessage
		if err := json.Unmarshal(jsonBytes, &raw); err != nil {
			t.Fatalf("json.Unmarshal()でエラーが発生: %v", err)
		}

		if _, ok := raw["duration_seconds"]; ok {
			t.Error("DurationSecondsが0の場合、JSONから省略されるべき")
		}
	})
}

// TestAlbumCreatedDataJSON はAlbumCreatedDataのJSONシリアライズを検証する。
func TestAlbumCreatedDataJSON(t *testing.T) {
	t.Parallel()

	data := AlbumCreatedData{
		UserID:      "user-abc",
		Name:        "旅行写真",
		Description: "2025年夏の旅行",
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("json.Marshal()でエラーが発生: %v", err)
	}

	var decoded AlbumCreatedData
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal()でエラーが発生: %v", err)
	}

	if decoded.UserID != data.UserID {
		t.Errorf("UserID = %q, want %q", decoded.UserID, data.UserID)
	}
	if decoded.Name != data.Name {
		t.Errorf("Name = %q, want %q", decoded.Name, data.Name)
	}
	if decoded.Description != data.Description {
		t.Errorf("Description = %q, want %q", decoded.Description, data.Description)
	}
}

// TestNotificationSentDataJSON はNotificationSentDataのJSONシリアライズを検証する。
func TestNotificationSentDataJSON(t *testing.T) {
	t.Parallel()

	data := NotificationSentData{
		UserID:  "user-xyz",
		Title:   "アップロード完了",
		Message: "メディアファイルのアップロードが完了しました",
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("json.Marshal()でエラーが発生: %v", err)
	}

	var decoded NotificationSentData
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal()でエラーが発生: %v", err)
	}

	if decoded.UserID != data.UserID {
		t.Errorf("UserID = %q, want %q", decoded.UserID, data.UserID)
	}
	if decoded.Title != data.Title {
		t.Errorf("Title = %q, want %q", decoded.Title, data.Title)
	}
	if decoded.Message != data.Message {
		t.Errorf("Message = %q, want %q", decoded.Message, data.Message)
	}
}

// TestMediaUploadCompensatedDataJSON はMediaUploadCompensatedDataのJSONシリアライズを検証する。
func TestMediaUploadCompensatedDataJSON(t *testing.T) {
	t.Parallel()

	data := MediaUploadCompensatedData{
		Reason: "処理に失敗したため補償",
		SagaID: "saga-001",
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("json.Marshal()でエラーが発生: %v", err)
	}

	var decoded MediaUploadCompensatedData
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal()でエラーが発生: %v", err)
	}

	if decoded.Reason != data.Reason {
		t.Errorf("Reason = %q, want %q", decoded.Reason, data.Reason)
	}
	if decoded.SagaID != data.SagaID {
		t.Errorf("SagaID = %q, want %q", decoded.SagaID, data.SagaID)
	}
}

// TestMediaDeletedDataJSON はMediaDeletedDataのJSONシリアライズを検証する。
func TestMediaDeletedDataJSON(t *testing.T) {
	t.Parallel()

	data := MediaDeletedData{
		UserID: "user-del",
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("json.Marshal()でエラーが発生: %v", err)
	}

	var decoded MediaDeletedData
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal()でエラーが発生: %v", err)
	}

	if decoded.UserID != data.UserID {
		t.Errorf("UserID = %q, want %q", decoded.UserID, data.UserID)
	}
}

// TestMediaProcessingFailedDataJSON はMediaProcessingFailedDataのJSONシリアライズを検証する。
func TestMediaProcessingFailedDataJSON(t *testing.T) {
	t.Parallel()

	data := MediaProcessingFailedData{
		Reason: "サムネイル生成に失敗しました",
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("json.Marshal()でエラーが発生: %v", err)
	}

	var decoded MediaProcessingFailedData
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal()でエラーが発生: %v", err)
	}

	if decoded.Reason != data.Reason {
		t.Errorf("Reason = %q, want %q", decoded.Reason, data.Reason)
	}
}

// TestAlbumDeletedDataJSON はAlbumDeletedDataのJSONシリアライズを検証する。
func TestAlbumDeletedDataJSON(t *testing.T) {
	t.Parallel()

	data := AlbumDeletedData{
		UserID: "user-del-album",
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("json.Marshal()でエラーが発生: %v", err)
	}

	var decoded AlbumDeletedData
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal()でエラーが発生: %v", err)
	}

	if decoded.UserID != data.UserID {
		t.Errorf("UserID = %q, want %q", decoded.UserID, data.UserID)
	}
}

// TestMediaAddedToAlbumDataJSON はMediaAddedToAlbumDataのJSONシリアライズを検証する。
func TestMediaAddedToAlbumDataJSON(t *testing.T) {
	t.Parallel()

	data := MediaAddedToAlbumData{
		MediaID: "media-add-1",
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("json.Marshal()でエラーが発生: %v", err)
	}

	var decoded MediaAddedToAlbumData
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal()でエラーが発生: %v", err)
	}

	if decoded.MediaID != data.MediaID {
		t.Errorf("MediaID = %q, want %q", decoded.MediaID, data.MediaID)
	}
}

// TestMediaRemovedFromAlbumDataJSON はMediaRemovedFromAlbumDataのJSONシリアライズを検証する。
func TestMediaRemovedFromAlbumDataJSON(t *testing.T) {
	t.Parallel()

	data := MediaRemovedFromAlbumData{
		MediaID: "media-rm-1",
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("json.Marshal()でエラーが発生: %v", err)
	}

	var decoded MediaRemovedFromAlbumData
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal()でエラーが発生: %v", err)
	}

	if decoded.MediaID != data.MediaID {
		t.Errorf("MediaID = %q, want %q", decoded.MediaID, data.MediaID)
	}
}
