package freejavbt

import "testing"

const sampleHTML = `<html><head>
<meta name="description" content="影片番號為SONE-001，影片名是SONE-001 テスト，發佈日期為2024-01-02，主演女優是女優A、女優B，影片時長120分鐘，屬於系列作，主題為巨乳、多P。">
<meta property="og:image" content="https://666.9989641.xyz/cover.jpg">
</head><body>
<img data-src="https://c0.jdbstatic.com/samples/d4/x_s_0.jpg">
<img data-src="https://c0.jdbstatic.com/samples/d4/x_s_1.jpg">
<table><tbody>
<tr><th>資訊</th><th class="date">更新日期</th><th class="copy">點擊復制</th></tr>
<tr><td><a class="magnet-name text-truncate" href="magnet:?xt=urn:btih:FECE34564104D670F1ED5D706EB337050B41C23A&amp;dn=[javdb.com]SONE-001-C.torrent">SONE-001-C</a><span class="tag bg-primary">高清</span><span class="tag bg-success">字幕</span><span class="tag bg-secondary"> 4個文件 </span></td><td class="date small"><span>2023-12-17</span></td></tr>
</tbody></table>
<h2 id="related-actress-title">同女優作品</h2><div class="d-flex flex-wrap"><a href="https://freejavbt.com/IPX-980"><div><img data-src="https://c0.jdbstatic.com/covers/j0/J0ymgq.jpg"></div></a></div>
<h2 id="related-genre-title">同主題作品</h2><div class="d-flex flex-wrap"><a href="https://freejavbt.com/OFTR-019"><div><img data-src="https://c0.jdbstatic.com/covers/j0/abc.jpg"></div></a></div>
</body></html>`

func TestParseDetail(t *testing.T) {
	got := parseDetail(sampleHTML)
	if got.Code != "SONE-001" || got.Title != "SONE-001 テスト" {
		t.Fatalf("code/title = %q / %q", got.Code, got.Title)
	}
	if got.ReleaseDate != "2024-01-02" || got.Duration != 120 {
		t.Fatalf("date/duration = %q / %d", got.ReleaseDate, got.Duration)
	}
	if len(got.Actors) != 2 || got.Actors[0] != "女優A" || got.Actors[1] != "女優B" {
		t.Fatalf("actors = %v", got.Actors)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "巨乳" {
		t.Fatalf("tags = %v", got.Tags)
	}
	if got.Cover != "https://666.9989641.xyz/cover.jpg" {
		t.Fatalf("cover = %q", got.Cover)
	}
	if len(got.Previews) != 2 {
		t.Fatalf("previews = %d", len(got.Previews))
	}
	if len(got.Magnets) != 1 {
		t.Fatalf("magnets = %d", len(got.Magnets))
	}
	m := got.Magnets[0]
	if m.Hash != "fece34564104d670f1ed5d706eb337050b41c23a" || !m.HasSubtitle || !m.HD || m.FilesCount != 4 || m.CreatedAt != "2023-12-17" || m.Name != "SONE-001-C" {
		t.Fatalf("magnet = %+v", m)
	}
	if len(got.ActorMovies) != 1 || got.ActorMovies[0].Code != "IPX-980" {
		t.Fatalf("actor movies = %+v", got.ActorMovies)
	}
	if len(got.RelatedMovies) != 1 || got.RelatedMovies[0].Code != "OFTR-019" {
		t.Fatalf("related movies = %+v", got.RelatedMovies)
	}
}

func TestParseDetailNoDescription(t *testing.T) {
	if got := parseDetail("<html></html>"); got.Title != "" || len(got.Magnets) != 0 {
		t.Fatalf("unexpected parse: %+v", got)
	}
}
