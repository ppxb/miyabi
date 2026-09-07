package service

import (
	"context"
	"encoding/json"
	"encoding/json/jsontext"
	"fmt"

	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/setting"
)

func loadSetting[T any](ctx context.Context, database *ent.Client, key string) (T, bool, error) {
	var value T
	record, err := database.Setting.Query().Where(setting.Key(key)).Only(ctx)
	if ent.IsNotFound(err) {
		return value, false, nil
	}
	if err != nil {
		return value, false, fmt.Errorf("load setting %s: %w", key, err)
	}
	if err := json.Unmarshal(record.Value, &value); err != nil {
		return value, false, fmt.Errorf("decode setting %s: %w", key, err)
	}
	return value, true, nil
}

func saveSetting(ctx context.Context, database *ent.Client, key string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode setting %s: %w", key, err)
	}
	if err := database.Setting.Create().SetKey(key).SetValue(jsontext.Value(encoded)).
		OnConflictColumns(setting.FieldKey).UpdateNewValues().Exec(ctx); err != nil {
		return fmt.Errorf("save setting %s: %w", key, err)
	}
	return nil
}
