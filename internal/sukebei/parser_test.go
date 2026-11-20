package sukebei

import "testing"

const sampleFeed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:nyaa="https://sukebei.nyaa.si/xmlns/nyaa">
  <channel>
    <title>Sukebei</title>
    <item>
      <title>[Studio] SSIS-001 1080p</title>
      <link>https://sukebei.nyaa.si/view/1</link>
      <pubDate>Wed, 01 Oct 2025 12:00:00 +0000</pubDate>
      <nyaa:infoHash>ABCDEF0123456789ABCDEF0123456789ABCDEF01</nyaa:infoHash>
      <nyaa:size>1.5 GiB</nyaa:size>
    </item>
    <item>
      <title>unrelated random title</title>
      <nyaa:infoHash>1111111111111111111111111111111111111111</nyaa:infoHash>
      <nyaa:size>500 MiB</nyaa:size>
    </item>
  </channel>
</rss>`

func TestIsCompilation(t *testing.T) {
	for _, title := range []string{"SSIS-001 4時間 BEST", "SSIS-001 合集", "ABC-001 Complete Collection", "XYZ-9 総集編"} {
		if !isCompilation(title) {
			t.Fatalf("expected compilation: %q", title)
		}
	}
	for _, title := range []string{"SSIS-001 1080p", "FC2-PPV-1234567", "SSIS-001 C subtitle"} {
		if isCompilation(title) {
			t.Fatalf("false positive: %q", title)
		}
	}
}

func TestParseFeed(t *testing.T) {
	entries, err := parseFeed([]byte(sampleFeed))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	first := entries[0]
	if first.InfoHash != "abcdef0123456789abcdef0123456789abcdef01" {
		t.Fatalf("hash = %q", first.InfoHash)
	}
	if first.Size != 1610612736 {
		t.Fatalf("size = %d, want 1610612736", first.Size)
	}
	if first.CreatedAt != "2025-10-01" {
		t.Fatalf("date = %q", first.CreatedAt)
	}
}
