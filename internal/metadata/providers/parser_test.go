package providers

import (
	"os"
	"testing"
)

func TestFC2ValidatesProductAndDoesNotInventHighResolutionImages(t *testing.T) {
	body := `<div class="items_article_headerInfo"><h3>Title<span style="display:none">Spam</span></h3><div class="items_article_softDevice"><p>商品ID: 1234567</p><p>販売日: 2025/03/04</p></div></div>
<div class="items_article_MainitemThumb"><span><img src="https://contents.fc2.com/small.jpg"></span></div><section class="items_article_SampleImages"><a href="https://contents.fc2.com/sample.jpg"><img src="thumbnail.jpg"></a></section>`
	result, err := parseFC2(body, "https://adult.contents.fc2.com/", "1234567")
	if err != nil {
		t.Fatal(err)
	}
	if result.Detail.Title != "Title" || result.Detail.Code != "FC2-1234567" || result.Detail.ReleaseDate != "2025-03-04" || len(result.Images) != 2 || result.Detail.Cover != "https://contents.fc2.com/small.jpg" {
		t.Fatalf("parse: %+v", result)
	}
	if _, err := parseFC2(body, "https://adult.contents.fc2.com/", "7654321"); err == nil {
		t.Fatal("accepted unrelated product ID")
	}
}

// Live response samples stay outside the repository, like real image fixtures.
func TestFC2LiveResponse(t *testing.T) {
	path := os.Getenv("MIYABI_TEST_FC2_PAGE")
	if path == "" {
		t.Skip("set MIYABI_TEST_FC2_PAGE to a downloaded FC2 product page")
	}
	id := os.Getenv("MIYABI_TEST_FC2_ID")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parseFC2(string(body), "https://adult.contents.fc2.com/", id)
	if err != nil {
		t.Fatal(err)
	}
	if result.Detail.Cover == "" || len(result.Images) == 0 {
		t.Fatal("live product has no images")
	}
}
