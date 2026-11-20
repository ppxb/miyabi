package javdb

import "testing"

const webSearchSample = `<div class="movie-list"><div class="item">
<a href="/v/ZY5eq" class="box" title="SSIS-001">
  <div class="video-title"><strong>SSIS-001</strong> Some title</div>
</a>
<a href="/v/ex3Qd" class="box" title="SSIS-001C">
  <div class="video-title"><strong>SSIS-001C</strong> Other</div>
</a>
</div></div>`

func TestParseWebSearch(t *testing.T) {
	results := parseWebSearch(webSearchSample)
	if len(results) != 2 || results[0].id != "ZY5eq" || results[0].code != "SSIS-001" || results[1].id != "ex3Qd" {
		t.Fatalf("results = %+v", results)
	}
}

func TestMatchWebResults(t *testing.T) {
	results := []webSearchResult{{id: "a", code: "ABP-123"}, {id: "b", code: "ABP-124"}}
	if id, err := matchWebResults(results, []string{"ABP-123"}, false); err != nil || id != "a" {
		t.Fatalf("exact match = %q %v", id, err)
	}
	if id, err := matchWebResults(results, []string{"ABP-999"}, false); err != nil || id != "" {
		t.Fatalf("miss = %q %v", id, err)
	}
	dup := []webSearchResult{{id: "a", code: "ABP-123"}, {id: "b", code: "ABP-123"}}
	if _, err := matchWebResults(dup, []string{"ABP-123"}, false); err == nil {
		t.Fatal("expected ambiguity error")
	}
}
