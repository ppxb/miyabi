package playback

import (
	"context"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
)

func TestCreateSessionRegistersSourcesAndRelease(t *testing.T) {
	service := &Service{sessions: make(map[string]*playSession), sessionTTL: time.Minute}
	source := domain.LibrarySource{AccountID: "100", Directory: domain.LibraryDirectory{ID: "10"}}
	result, err := service.createSession(source, []mediaSpec{
		{address: "https://cdn.example/a.mp4", mediaType: "video/mp4", label: "原画"},
		{address: "https://cdn.example/master.m3u8", mediaType: "application/x-mpegurl", label: "1080p", playlist: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ID == "" || len(result.Sources) != 2 {
		t.Fatalf("result = %+v", result)
	}
	if result.Sources[0].Src != "/api/play/"+result.ID+"/stream/0" || result.Sources[0].Type != "video/mp4" {
		t.Fatalf("first source = %+v", result.Sources[0])
	}
	if result.Sources[1].Src != "/api/play/"+result.ID+"/stream/1" || result.Sources[1].Type != "application/x-mpegurl" {
		t.Fatalf("second source = %+v", result.Sources[1])
	}
	if _, exists := service.sessions[result.ID]; !exists {
		t.Fatal("session not stored")
	}
	service.Release(result.ID)
	if _, exists := service.sessions[result.ID]; exists {
		t.Fatal("session not released")
	}
}

func TestResourceReportsEndedSession(t *testing.T) {
	service := &Service{sessions: make(map[string]*playSession)}
	if _, _, err := service.resource(context.Background(), "missing", 0); !domain.IsKind(err, domain.KindNotFound) {
		t.Fatalf("error = %v, want not-found", err)
	}
}
