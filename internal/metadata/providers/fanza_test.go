package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	_ "image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/netx"
	"golang.org/x/time/rate"
)

type fanzaRequest struct {
	Query     string
	Variables map[string]any
}

func testFANZA(t *testing.T, handler func(fanzaRequest) any) *FANZA {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Fanza-Device") != "BROWSER" {
			t.Error("incorrect GraphQL request")
		}
		var request fanzaRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(handler(request))
	}))
	t.Cleanup(server.Close)
	s := NewFANZA(nil)
	s.endpoint, s.http = server.URL, server.Client()
	s.limiter = rate.NewLimiter(rate.Inf, 1)
	t.Cleanup(s.Close)
	return s
}

func TestFANZAResolvesOfficialIdentityAcrossPagesAndMapsMetadata(t *testing.T) {
	var offsets []float64
	s := testFANZA(t, func(r fanzaRequest) any {
		switch {
		case strings.Contains(r.Query, "query MovieSearch"):
			if r.Variables["keyword"] != "ssis00001" {
				t.Errorf("keyword: %v", r.Variables)
			}
			offset := r.Variables["offset"].(float64)
			offsets = append(offsets, offset)
			id := "unrelated"
			if offset == 40 {
				id = "unpredictable-content-id"
			}
			return map[string]any{"data": map[string]any{"legacySearchPPV": map[string]any{"result": map[string]any{
				"contents": []any{map[string]any{"id": id}}, "pageInfo": map[string]any{"hasNext": offset == 0},
			}}}}
		case strings.Contains(r.Query, "query MovieIdentities"):
			code := "SSIS-002"
			if r.Variables["id0"] == "unpredictable-content-id" {
				code = "SSIS-001"
			}
			return map[string]any{"data": map[string]any{"m0": map[string]any{"id": r.Variables["id0"], "makerContentId": code}}}
		default:
			if r.Variables["id"] != "unpredictable-content-id" {
				t.Error("guessed content ID")
			}
			return json.RawMessage(`{"data":{"ppvContent":{
			"id":"unpredictable-content-id","makerContentId":"SSIS-001","floor":"AV","title":"Official title",
			"duration":8826,"deliveryStartDate":"2021-02-18T01:00:00Z","makerReleasedAt":"2021-02-18T15:00:00Z",
			"packageImage":{"largeUrl":"https://pics.dmm.co.jp/cover.jpg","mediumUrl":"https://pics.dmm.co.jp/thumb.jpg"},
			"sampleImages":[{"imageUrl":"small.jpg","largeImageUrl":"original.jpg"}],
			"actresses":[{"id":"a1","name":"Actor","imageUrl":"avatar.jpg"}],"histrions":[{"id":"a2","name":"Actor two"}],
			"maker":{"id":"m1","name":"Studio"},"series":{"id":"s1","name":"Series"},"directors":[{"id":"d1","name":"Director"}],
			"genres":[{"id":"g1","name":"Genre"}]
			},"reviewSummary":{"average":4.32}}}`)
		}
	})
	got, err := s.Fetch(t.Context(), domain.MovieRef{Code: "SSIS-001"})
	if err != nil {
		t.Fatal(err)
	}
	m := got.Detail
	if !reflect.DeepEqual(offsets, []float64{0, 40}) || m.ID != "" || m.Sources[0].ID != "unpredictable-content-id" || m.ReleaseDate != "2021-02-19" || m.Duration != 147 || m.RatingMax != 5 || m.RatingSource != "fanza" {
		t.Fatalf("identity or metadata: %+v offsets=%v", m, offsets)
	}
	if len(m.Actors) != 2 || m.Actors[1].Gender != "male" || m.Maker.Provider != "fanza" || m.Tags[0].Provider != "fanza" || len(got.Images) != 3 || m.PreviewImages[0].Original != "original.jpg" {
		t.Fatalf("mapping: %+v", got)
	}
	if m.PreviewVideo != "" {
		t.Fatal("unverified trailer exposed")
	}
}

func TestFANZAMatchingRejectsAmbiguityAndNeverAcceptsCloseNumbers(t *testing.T) {
	for _, tc := range []struct {
		name     string
		items    []fanzaIdentity
		want     string
		conflict bool
	}{
		{"exact beats zero padding", []fanzaIdentity{{"other", "ABP-00001"}, {"exact", "ABP-001"}}, "exact", false},
		{"same ID duplicated", []fanzaIdentity{{"same", "ABP-001"}, {"same", "ABP-001"}}, "same", false},
		{"distinct releases", []fanzaIdentity{{"a", "ABP-001"}, {"b", "ABP-001"}}, "", true},
		{"wrong number", []fanzaIdentity{{"a", "ABP-002"}}, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id, err := matchFANZA(tc.items, "ABP-001")
			if id != tc.want || (domain.KindOf(err) == domain.KindConflict) != tc.conflict {
				t.Fatalf("id=%s err=%v", id, err)
			}
		})
	}
}

func TestFANZAErrorsAndMalformedResponsesAreNotNegativeResults(t *testing.T) {
	for _, body := range []string{`{"errors":[{"message":"rate limited"}],"data":null}`, `{"data":null}`, `{"data":{"legacySearchPPV":null}}`, `{"data":{"legacySearchPPV":{"result":{"contents":[]}}}}`} {
		t.Run(body, func(t *testing.T) {
			s := testFANZA(t, func(fanzaRequest) any { return json.RawMessage(body) })
			_, err := s.Fetch(t.Context(), domain.MovieRef{Code: "ABP-001"})
			if err == nil || errors.Is(err, metadata.ErrNotFound) {
				t.Fatalf("malformed response treated as miss: %v", err)
			}
		})
	}
}

func TestFANZACompactSearchAndAmateurImageFallback(t *testing.T) {
	var keywords []string
	s := testFANZA(t, func(r fanzaRequest) any {
		switch {
		case strings.Contains(r.Query, "MovieSearch"):
			keyword := r.Variables["keyword"].(string)
			keywords = append(keywords, keyword)
			contents := []any{}
			if keyword == "fuyu079" {
				contents = append(contents, map[string]string{"id": "fuyu079"})
			}
			return map[string]any{"data": map[string]any{"legacySearchPPV": map[string]any{"result": map[string]any{"contents": contents, "pageInfo": map[string]any{"hasNext": false}}}}}
		case strings.Contains(r.Query, "MovieIdentities"):
			return json.RawMessage(`{"data":{"m0":{"id":"fuyu079","makerContentId":"FUYU-079"}}}`)
		default:
			return json.RawMessage(`{"data":{"ppvContent":{"id":"fuyu079","makerContentId":"FUYU-079","floor":"AMATEUR","title":"Title","packageImage":{"largeUrl":null,"mediumUrl":"small.jpg"},"amateurActress":{"id":"a1","name":"Actor"}}}}`)
		}
	})
	m, err := s.Fetch(t.Context(), domain.MovieRef{Code: "FUYU-079"})
	if err != nil || !reflect.DeepEqual(keywords, []string{"fuyu00079", "fuyu079"}) || len(m.Images) != 1 || m.Images[0].URL != "small.jpg" || len(m.Detail.Actors) != 1 {
		t.Fatalf("result=%+v err=%v keywords=%v", m, err, keywords)
	}
}

func TestFANZALive(t *testing.T) {
	codes := os.Getenv("MIYABI_TEST_FANZA_CODES")
	if codes == "" {
		t.Skip("set MIYABI_TEST_FANZA_CODES for live verification; images remain outside the repository")
	}
	proxy, err := netx.NewProxyManager(netx.ProxyConfig{Enabled: os.Getenv("MIYABI_TEST_PROXY") != "", URL: os.Getenv("MIYABI_TEST_PROXY")})
	if err != nil {
		t.Fatal(err)
	}
	s := NewFANZA(proxy)
	defer s.Close()
	for _, code := range strings.Split(codes, ",") {
		t.Run(code, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
			defer cancel()
			m, err := s.Fetch(ctx, domain.MovieRef{Code: code})
			if err != nil {
				t.Fatal(err)
			}
			if m.Detail.ID != "" || !codeid.IsFormatEquivalent(m.Detail.Code, code) || len(m.Images) == 0 {
				t.Fatalf("invalid identity: %+v", m.Detail.Sources)
			}
			media, err := s.Media(ctx, m.Images[0].URL)
			if err != nil {
				t.Fatal(err)
			}
			decoded, _, err := image.Decode(bytes.NewReader(media.Body))
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("%s: %s, cover %dx%d, previews %d", code, m.Detail.Sources[0].ID, decoded.Bounds().Dx(), decoded.Bounds().Dy(), len(m.Detail.PreviewImages))
		})
	}
}
