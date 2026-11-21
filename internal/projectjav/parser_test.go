package projectjav

import "testing"

const searchFixture = `<html><body>
<div class="col-md-4 mb-4 video-item" data-movie-id="237357">
    <a href="/movie/jqj-007-237357">cover</a>
    <table class="table table-sm mt-2">
        <tr>
            <td><a href="magnet:?xt=urn:btih:1dd2e27d2846d5823dcf6fea3db7b8a0484bbe2c&amp;amp;dn=JQJ-007%20%5BHD%5D%20sample&amp;amp;tr=http%3A%2F%2Ftracker" class="magnet-click-track">Magnet</a></td>
            <td><i class="fas fa-weight-hanging"></i> 5.2gb</td>
            <td><i class="far fa-calendar-alt"></i> 20/11/26</td>
        </tr>
    </table>
</div>
<div class="col-md-4 mb-4 video-item" data-movie-id="237426">
    <a href="/movie/jqj-004-237426">cover</a>
    <table class="table table-sm mt-2">
        <tr>
            <td><a href="magnet:?xt=urn:btih:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&amp;amp;dn=JQJ-004%20other">Magnet</a></td>
        </tr>
    </table>
</div>
</body></html>`

func TestParseSearchMatchesCode(t *testing.T) {
	magnets := parseSearch([]byte(searchFixture), "JQJ-007")
	if len(magnets) != 1 {
		t.Fatalf("magnets = %d, want 1: %+v", len(magnets), magnets)
	}
	got := magnets[0]
	if got.Hash != "1dd2e27d2846d5823dcf6fea3db7b8a0484bbe2c" {
		t.Errorf("hash = %q", got.Hash)
	}
	if got.Name != "JQJ-007 [HD] sample" {
		t.Errorf("name = %q", got.Name)
	}
	gib := float64(1 << 30)
	if got.Size != int64(5.2*gib) {
		t.Errorf("size = %d", got.Size)
	}
	if got.CreatedAt != "2026-11-20" {
		t.Errorf("created_at = %q", got.CreatedAt)
	}
	if len(got.Sources) != 1 || got.Sources[0] != "projectjav" {
		t.Errorf("sources = %+v", got.Sources)
	}
}

func TestParseSizeUnits(t *testing.T) {
	cases := map[string]int64{
		`<i class="fas fa-weight-hanging"></i> 1gb`:   1 << 30,
		`<i class="fas fa-weight-hanging"></i> 512mb`: 512 << 20,
		`<i class="fas fa-weight-hanging"></i> 2tb`:   2 << 40,
	}
	for cell, want := range cases {
		if got := parseSize(cell); got != want {
			t.Errorf("parseSize(%q) = %d, want %d", cell, got, want)
		}
	}
}
