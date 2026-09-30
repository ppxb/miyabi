package providers

import (
	"os"
	"testing"
)

func TestJavBusRecordedPageUsesSourceIdentitiesAndOriginalImages(t *testing.T) {
	body, err := os.ReadFile("../../javbus/testdata/detail_ssis-001.html")
	if err != nil {
		t.Fatal(err)
	}
	result, err := parseJavBus(string(body), "https://www.javbus.com/")
	if err != nil {
		t.Fatal(err)
	}
	if result.Detail.Code != "SSIS-001" || result.Detail.Duration != 150 || len(result.Detail.Actors) != 2 || result.Detail.Sources[0].Provider != "javbus" || result.Detail.Actors[0].Provider != "javbus" || len(result.Images) < 2 {
		t.Fatalf("incomplete page: %+v", result)
	}
	if result.Images[0].URL != "https://www.javbus.com/pics/cover/83ie_b.jpg" {
		t.Fatalf("not full cover: %+v", result.Images[0])
	}
}

func TestMGStageParsesProductIdentityAndFullSizeLinks(t *testing.T) {
	result, err := parseMGStage(`<div id="center_column"><div><h1>Title</h1></div><div class="detail_data">
<a id="EnlargeImage" href="https://image.mgstage.com/full.jpg">Cover</a>
<table><tr><th>品番：</th><td>300MIUM-001</td></tr><tr><th>出演：</th><td><a href="/search/?actor[]=123">Actor</a></td></tr>
<tr><th>収録時間：</th><td>120分</td></tr><tr><th>商品発売日：</th><td>2025/02/03</td></tr></table></div>
<dl id="sample-photo"><dd><ul><li><a href="https://image.mgstage.com/original.jpg"><img src="small.jpg"></a></li></ul></dd></dl></div>`, "https://www.mgstage.com/")
	if err != nil {
		t.Fatal(err)
	}
	if result.Detail.Code != "300MIUM-001" || result.Detail.Duration != 120 || result.Detail.ReleaseDate != "2025-02-03" || len(result.Detail.Actors) != 1 || result.Detail.Actors[0].ID != "123" || len(result.Images) != 2 || result.Images[1].URL != "https://image.mgstage.com/original.jpg" {
		t.Fatalf("parse: %+v", result)
	}
	if _, err := parseMGStage(`<h1>Access denied</h1>`, "https://www.mgstage.com/"); err == nil {
		t.Fatal("challenge treated as product")
	}
}

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
