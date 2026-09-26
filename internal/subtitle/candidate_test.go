package subtitle

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestXunleiSearchParsing(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code": 0,
			"data": [
				{"name": "SSIS-589.chs.srt", "url": "http://example.com/ssis589.srt", "ext": "srt", "languages": ["zh-CN"]},
				{"name": "SSIS-589.uncensored.ass", "url": "http://example.com/ssis589-uncensored.ass", "ext": "ass", "languages": ["繁体"]},
				{"name": "SSIS-589.srt", "url": "http://example.com/ssis589-plain.srt", "ext": "", "languages": []}
			]
		}`))
	}))
	defer ts.Close()

	provider := NewXunleiProvider(nil)
	provider.endpoint = ts.URL
	results, err := provider.Search(context.Background(), "SSIS-589")
	if err != nil {
		t.Fatal(err)
	}
	want := []Candidate{
		{Provider: "迅雷", Name: "SSIS-589.chs.srt", URL: "http://example.com/ssis589.srt", Format: "srt", Language: LangSimplifiedChinese, Version: VersionStandard},
		{Provider: "迅雷", Name: "SSIS-589.uncensored.ass", URL: "http://example.com/ssis589-uncensored.ass", Format: "ass", Language: LangTraditionalChinese, Version: VersionUncensored},
		{Provider: "迅雷", Name: "SSIS-589.srt", URL: "http://example.com/ssis589-plain.srt", Format: "srt", Language: LangUnknown, Version: VersionStandard},
	}
	if len(results) != len(want) {
		t.Fatalf("results = %+v", results)
	}
	for i := range want {
		if results[i] != want[i] {
			t.Errorf("result %d = %+v; want %+v", i, results[i], want[i])
		}
	}
}

func TestSubtitleCatHTMLParsing(t *testing.T) {
	searchHTML := `
	<table class="sub-table">
		<tbody>
			<tr>
				<td><a href="/subs/123/movie.html">SSIS-589 Subtitle</a></td>
			</tr>
		</tbody>
	</table>`

	entries := parseSubtitleCatSearch([]byte(searchHTML), "https://www.subtitlecat.com")
	if len(entries) != 1 || entries[0].Name != "SSIS-589 Subtitle" {
		t.Fatalf("entries = %+v", entries)
	}

	detailHTML := `
	<div class="sub-single">
		<img class="flag" alt="zh-CN" />
		<a class="green-link" href="https://www.subtitlecat.com/download/123.srt">Download</a>
	</div>
	<div class="sub-single">
		<img class="flag" alt="zh-TW" />
		<a class="green-link" href="/download/124.srt">Download</a>
	</div>
	<div class="sub-single">
		<img class="flag" alt="en" />
		<a class="green-link" href="/download/125.srt">Download</a>
	</div>`

	items := parseSubtitleCatDetail([]byte(detailHTML), "SSIS-589 Subtitle", "https://www.subtitlecat.com")
	if len(items) != 2 {
		t.Fatalf("expected simplified and traditional items, got %+v", items)
	}
	if items[0].Language != LangSimplifiedChinese || items[1].Language != LangTraditionalChinese ||
		items[1].URL != "https://www.subtitlecat.com/download/124.srt" || items[0].Format != "srt" {
		t.Errorf("items = %+v", items)
	}
}

func TestRankKeepsOnlyTheMovieAndOrdersByRelevance(t *testing.T) {
	candidates := []Candidate{
		{Name: "SSIS-5890.chs.srt", Format: "srt", Language: LangSimplifiedChinese, Version: VersionStandard},
		{Name: "OTHER-589.chs.srt", Format: "srt", Language: LangSimplifiedChinese, Version: VersionStandard},
		{Name: "Unrelated title.srt", Format: "srt", Language: LangSimplifiedChinese, Version: VersionStandard},
		{Name: "SSIS-589.idx", Format: "", Language: LangSimplifiedChinese, Version: VersionStandard},
		{Name: "SSIS-589.uncensored.chs.srt", Format: "srt", Language: LangSimplifiedChinese, Version: VersionUncensored},
		{Name: "ssis00589.cht.ass", Format: "ass", Language: LangTraditionalChinese, Version: VersionStandard},
		{Name: "SSIS-589-C.chs.srt", Format: "srt", Language: LangSimplifiedChinese, Version: VersionStandard},
	}

	censored := Rank(candidates, "SSIS-589", false)
	if len(censored) != 2 || censored[0].Name != "SSIS-589-C.chs.srt" || censored[1].Name != "ssis00589.cht.ass" {
		t.Fatalf("censored ranking = %+v", censored)
	}

	uncensored := Rank(candidates, "SSIS-589", true)
	if len(uncensored) != 3 || uncensored[0].Version != VersionUncensored {
		t.Fatalf("uncensored ranking = %+v", uncensored)
	}

	// Release labels may carry distributor digits the catalogue number omits.
	labelled := Rank([]Candidate{{Name: "LUXU-1899.srt", Format: "srt", Version: VersionStandard}}, "259LUXU-1899", false)
	if len(labelled) != 1 {
		t.Fatalf("label-prefixed catalogue number did not match: %+v", labelled)
	}
}
