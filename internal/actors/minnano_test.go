package actors

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	cloudscraper "github.com/Advik-B/cloudscraper/lib"
	"github.com/Advik-B/cloudscraper/lib/stealth"
)

func testMinNano(t *testing.T, handler http.HandlerFunc) *MinNano {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	scraper, err := cloudscraper.New(cloudscraper.WithStealth(stealth.Options{Enabled: false}))
	if err != nil {
		t.Fatal(err)
	}
	return &MinNano{base: server.URL + "/", scraper: scraper}
}

func minnanoProfileHTML(id, name string) string {
	return `<!doctype html><html><head>
<link rel="canonical" href="https://www.minnano-av.com/actress` + id + `.html">
<meta property="og:url" content="https://www.minnano-av.com/actress` + id + `.html">
</head><body>
<img src="/p_actress_125_125/013/` + id + `.jpg?newav" title="` + name + `">
<div class="act-profile"><table>
<tr><td><h2>` + name + ` （みかみゆあ / Mikami Yua）</h2></td></tr>
<tr><td><span>別名</span><p>鬼頭桃菜(SKE48チームE) （ / ）</p></td></tr>
<tr><td><span>生年月日</span><p>1993年08月16日 （現在 33歳）しし座</p></td></tr>
<tr><td><span>サイズ</span><p>T159 / B88(<a href="actress_list.php?cup=G">Gカップ</a>) / W57 / H83 / S23.5</p></td></tr>
<tr><td><span>血液型</span><p><a href="actress_list.php?blood_type=A">A型</a></p></td></tr>
<tr><td><span>出身地</span><p><a href="actress_list.php?place=23">愛知県</a></p></td></tr>
<tr><td><span>趣味・特技</span><p>アダルトDVD鑑賞・中国語、韓国語</p></td></tr>
<tr><td><span>デビュー作品</span><p>Princess Peach ` + name + `（2015年06月 01日）</p></td></tr>
</table></div>
</body></html>`
}

func TestMinNanoLookupParsesProfile(t *testing.T) {
	m := testMinNano(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/search_result.php") {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(minnanoProfileHTML("681074", "三上悠亜")))
	})

	p, err := m.Lookup(t.Context(), "三上悠亜")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "三上悠亜" || p.SourceID != "681074" {
		t.Fatalf("name/id = %q/%q", p.Name, p.SourceID)
	}
	if !slices.Equal(p.Aliases, []string{"鬼頭桃菜"}) {
		t.Fatalf("aliases = %v", p.Aliases)
	}
	if p.Birthday != "1993-08-16" || p.DebutDate != "2015-06-01" {
		t.Fatalf("birthday/debut = %q/%q", p.Birthday, p.DebutDate)
	}
	if p.Height != 159 || p.CupSize != "G" || p.Measurements != "B:88 / W:57 / H:83" {
		t.Fatalf("sizes = %d %q %q", p.Height, p.CupSize, p.Measurements)
	}
	if p.BloodType != "A" || p.Nationality != "愛知県" || p.Hobby != "アダルトDVD鑑賞・中国語、韓国語" {
		t.Fatalf("bio = %q %q %q", p.BloodType, p.Nationality, p.Hobby)
	}
	wantImage := m.base + "p_actress_125_125/013/681074.jpg?newav"
	if len(p.Images) != 1 || p.Images[0] != wantImage {
		t.Fatalf("images = %v", p.Images)
	}
}

func TestMinNanoLookupFollowsListPage(t *testing.T) {
	m := testMinNano(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search_result.php":
			_, _ = w.Write([]byte(`<html><body><table class="tbllist actress">
<tr><td><h2><a href="actress872837.html">相澤麗羅</a></h2></td></tr>
<tr><td><h2><a href="actress999999.html">相澤麗子</a></h2></td></tr>
</table></body></html>`))
		case "/actress872837.html":
			_, _ = w.Write([]byte(minnanoProfileHTML("872837", "相澤麗羅")))
		default:
			http.NotFound(w, r)
		}
	})

	p, err := m.Lookup(t.Context(), "相澤 麗羅")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "相澤麗羅" || p.SourceID != "872837" {
		t.Fatalf("profile = %q/%q", p.Name, p.SourceID)
	}
}

// A valid result list without an exact name match is a miss, not a retryable
// challenge.
func TestMinNanoLookupListWithoutExactMatch(t *testing.T) {
	m := testMinNano(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search_result.php" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`<html><body><table class="tbllist actress">
<tr><td><h2><a href="actress999999.html">相澤麗子</a></h2></td></tr>
</table></body></html>`))
	})
	if _, err := m.Lookup(t.Context(), "三上悠亜"); !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("err = %v, want ErrProviderNotFound", err)
	}
}

func TestMinNanoLookupZeroResults(t *testing.T) {
	m := testMinNano(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body><table class="tbllist actress"></table>
<p>女優検索結果。0名のAV女優が見つかりました</p></body></html>`))
	})
	if _, err := m.Lookup(t.Context(), "三上悠亚"); !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("err = %v, want ErrProviderNotFound", err)
	}
}

// A challenge or layout change after a search hit is retryable, not a miss.
func TestMinNanoLookupUnrecognizedPageIsAnError(t *testing.T) {
	m := testMinNano(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><head><title>Just a moment...</title></head><body></body></html>`))
	})
	if _, err := m.Lookup(t.Context(), "三上悠亜"); err == nil || errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("err = %v, want a retryable error", err)
	}
}
