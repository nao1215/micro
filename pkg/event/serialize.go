package event

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// New は新しいイベントを生成する。
// dataにはイベント固有のデータ構造体を渡す。JSON形式にシリアライズされる。
func New(aggregateID string, aggregateType AggregateType, eventType Type, version int64, data any) (*Event, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("イベントデータのシリアライズに失敗: %w", err)
	}

	return &Event{
		ID:            uuid.New().String(),
		AggregateID:   aggregateID,
		AggregateType: aggregateType,
		EventType:     eventType,
		Data:          jsonData,
		Version:       version,
		CreatedAt:     time.Now().UTC(),
	}, nil
}

// DecodeData はイベントのDataフィールドを指定された型にデシリアライズする。
func DecodeData[T any](e *Event) (*T, error) {
	var data T
	if err := json.Unmarshal(e.Data, &data); err != nil {
		return nil, fmt.Errorf("イベントデータのデシリアライズに失敗: %w", err)
	}
	return &data, nil
}
