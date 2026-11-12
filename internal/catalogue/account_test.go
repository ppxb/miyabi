package catalogue

import (
	"encoding/json"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/ent/setting"
	"github.com/ppxb/miyabi/internal/javdb"
)

func TestJavDBAccountRoundTrip(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	service, err := New(t.Context(), store.Client, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()

	initial, err := service.JavDBConfig(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if initial.Configured() || initial.Username != "" {
		t.Fatalf("initial config = %#v", initial)
	}

	if err := service.UpdateJavDBConfig(t.Context(), javdb.Config{Username: " user ", Password: " pass "}); err != nil {
		t.Fatal(err)
	}
	updated, err := service.JavDBConfig(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if updated.Username != "user" || updated.Password != "pass" {
		t.Fatalf("updated config = %#v", updated)
	}

	record, err := store.Client.Setting.Query().Where(setting.Key(javdbAccountSetting)).Only(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var saved javdb.Config
	if err := json.Unmarshal(record.Value, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Username != "user" || saved.Password != "pass" {
		t.Fatalf("saved config = %#v", saved)
	}

	if err := service.UpdateJavDBConfig(t.Context(), javdb.Config{Username: "only"}); err == nil {
		t.Fatal("username without password must be rejected")
	}
}
