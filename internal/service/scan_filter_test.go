package service

import (
	"context"
	"fmt"
	"path"
	"testing"

	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/nfo"
	"github.com/ppxb/miyabi/internal/pan"
)

type scanMetadataClient struct {
	panClient
	bodies map[string][]byte
	reads  int
}

func (client *scanMetadataClient) ReadMetadata(_ context.Context, _, pickCode string, _ int64) ([]byte, error) {
	client.reads++
	body, ok := client.bodies[pickCode]
	if !ok {
		return nil, fmt.Errorf("unexpected metadata read %q", pickCode)
	}
	return body, nil
}

// Reproduce the reported 14 movies / 27 videos, including existing incorrect
// associations that already acquired a JavDB ID through a directory NFO.
func TestRescanRepairsAuxiliaryVideosAndSSNISubtitleAlias(t *testing.T) {
	library, client := panConcurrencyFixture(t)
	ctx := t.Context()
	source := library.drive.snapshot().source()
	records := []struct {
		path, code string
		size       int64
	}{
		{"JUR-824/JUR-824.mp4", "JUR-824", 1371929265},
		{"JUR-824/manko.fun.mp4", "JUR-824", 11057812},
		{"259LUXU-1899/489155.com@259LUXU-1899.mp4", "LUXU-1899", 3117012923},
		{"259LUXU-1899/社 區 最 新 情 報.mp4", "LUXU-1899", 15089802},
		{"259LUXU-1899/台湾uu美少女直播 20年信誉保证服务全球.mp4", "LUXU-1899", 13890047},
		{"CNSTV-031/CNSTV-031.mp4", "CNSTV-031", 1610500229},
		{"CNSTV-031/manko.fun.mp4", "CNSTV-031", 11057812},
		{"090826_100-PACO/4k688.com@090826_100-PACO.mp4", "090826-100", 1836294932},
		{"PRWF-015-U/489155.com@PRWF-015-U.mp4", "PRWF-015", 3166241936},
		{"PRWF-015-U/社 區 最 新 情 報.mp4", "PRWF-015", 15089802},
		{"PRWF-015-U/台湾uu美少女直播 20年信誉保证服务全球.mp4", "PRWF-015", 13890047},
		{"FC2PPV-4973170/489155.com@FC2PPV-4973170.mp4", "FC2-PPV-4973170", 7193439652},
		{"FC2PPV-4973170/社 區 最 新 情 報.mp4", "FC2-PPV-4973170", 15089802},
		{"FC2PPV-4973170/台湾uu美少女直播 20年信誉保证服务全球.mp4", "FC2-PPV-4973170", 13890047},
		{"HEZ-916/489155.com@HEZ-916.mp4", "HEZ-916", 8910846876},
		{"HEZ-916/社 區 最 新 情 報.mp4", "HEZ-916", 15089802},
		{"HEZ-916/台湾uu美少女直播 20年信誉保证服务全球.mp4", "HEZ-916", 13890047},
		{"ssni-574-C/ssni-574-C.mp4", "SSNI-574", 5219311140},
		{"ssni-574-C/UUE29.mp4", "UUE-29", 65753642},
		{"SSNI748C/SSNI748C.mp4", "SSNI-748C", 1242774057},
		{"SSNI748C/点击观看 房间火爆  uup87.mp4", "UUP-87", 52432612},
		{"SSNI748C/★APP~5526apk.mp4", "", 12146689},
		{"SSNI748C/★看~prpxv.mp4", "", 12141743},
		{"SSNI748C/最新情報.mp4", "", 11990579},
		{"SSNI748C/更刺激的都在這~/APP.mp4", "SSNI-748", 3611254},
		{"SDNM-557/SDNM-557.mp4", "SDNM-557", 1614621142},
		{"EMBZ-349/EMBZ-349.mp4", "EMBZ-349", 1460605388},
	}
	movies := make(map[string]*ent.Movie)
	entries := map[string][]pan.File{"10": {}}
	directories := map[string]string{".": "10"}
	for index, entry := range records {
		parent := path.Dir(entry.path)
		if _, exists := directories[parent]; !exists {
			parentID := fmt.Sprintf("dir-%d", len(directories))
			directories[parent] = parentID
			ancestorID := directories[path.Dir(parent)]
			entries[ancestorID] = append(entries[ancestorID], pan.File{ID: parentID, Name: path.Base(parent), IsDirectory: true})
			entries[parentID] = []pan.File{{ID: "nfo-" + parentID, Name: "movie.nfo", PickCode: "nfo-" + parentID}}
		}
		id, parentID := fmt.Sprintf("video-%d", index), directories[parent]
		video := pan.File{ID: id, ParentID: parentID, Name: path.Base(entry.path), Size: entry.size, SHA1: "hash-" + id}
		entries[parentID] = append(entries[parentID], video)
		create := library.database.File.Create().SetFileID(id).SetParentID(parentID).SetName(video.Name).
			SetSize(video.Size).SetSha1(video.SHA1).SetAccountID(source.AccountID).SetRootID(source.Directory.ID).
			SetPath(path.Join(source.Directory.Path, entry.path))
		if entry.code != "" {
			if movies[entry.code] == nil {
				builder := library.database.Movie.Create().SetCode(entry.code)
				if entry.code == "UUE-29" || entry.code == "UUP-87" || entry.code == "SSNI-748C" {
					builder.SetScrapeStatus(movie.ScrapeStatusFailed)
				} else {
					builder.SetJavdbID("catalogue-" + entry.code).SetScrapeStatus(movie.ScrapeStatusDone)
				}
				movies[entry.code] = builder.SaveX(ctx)
			}
			create.SetMovie(movies[entry.code])
		}
		create.SaveX(ctx)
	}
	client.list = func(_ context.Context, _, id string, offset, limit int) (pan.FilePage, error) {
		if offset != 0 {
			return pan.FilePage{}, fmt.Errorf("unexpected offset %d", offset)
		}
		return pan.FilePage{Files: entries[id], Total: len(entries[id]), Path: []pan.Directory{{ID: source.Directory.ID}}}, nil
	}
	metadata := &scanMetadataClient{panClient: client}
	library.drive.client = metadata
	queued := library.database.Task.Query().Where(task.TypeEQ("scan")).OnlyX(ctx)
	if err := library.Scan(ctx, TaskJob{ID: queued.ID, Payload: queued.Payload}); err != nil {
		t.Fatal(err)
	}
	page, err := library.Movies(ctx, 1, 24)
	if err != nil || page.Total != 11 || library.database.File.Query().CountX(ctx) != 27 ||
		library.database.File.Query().Where(file.MovieIDIsNil()).CountX(ctx) != 16 {
		t.Fatalf("rescan did not repair the reported library: %+v err=%v", page, err)
	}
	if metadata.reads != 0 {
		t.Fatal("auxiliary files triggered directory NFO inference")
	}
	for _, code := range []string{"SSNI-748C", "UUE-29", "UUP-87"} {
		if library.database.Movie.Query().Where(movie.CodeEQ(code)).ExistX(ctx) {
			t.Fatalf("incorrect movie %s survived rescan", code)
		}
	}
	canonical := library.database.Movie.Query().Where(movie.CodeEQ("SSNI-748")).OnlyX(ctx)
	if canonical.ID != movies["SSNI-748"].ID || valueOrZero(canonical.JavdbID) != "catalogue-SSNI-748" {
		t.Fatal("subtitle alias repair replaced valid catalogue metadata")
	}
	files := library.database.File.Query().Where(file.MovieIDEQ(canonical.ID)).AllX(ctx)
	if len(files) != 1 || files[0].Name != "SSNI748C.mp4" {
		t.Fatalf("SSNI-748 playback still points at an auxiliary file: %+v", files)
	}
	for _, job := range library.database.Task.Query().Where(task.TypeEQ("scrape")).AllX(ctx) {
		input, err := decodeTaskPayload[metadataPayload](job.Payload)
		if err != nil {
			t.Fatal(err)
		}
		if code := input.Code; code == "UUE-29" || code == "UUP-87" || code == "SSNI-748C" {
			t.Fatalf("incorrect movie was queued again: %v", code)
		}
	}
}

func TestNFOIdentifiesOnlyEligibleVideosAndSmallFilesDoNotMakeDirectoryShared(t *testing.T) {
	library, client := panConcurrencyFixture(t)
	ctx := t.Context()
	source := library.drive.snapshot().source()
	body, err := nfo.Encode(nfo.Movie{Code: "ABP-001", Title: "Fixture"})
	if err != nil {
		t.Fatal(err)
	}
	entries := []pan.File{
		{ID: "feature", ParentID: "10", Name: "feature.mp4", Size: 1 << 30, SHA1: "feature"},
		{ID: "auxiliary", ParentID: "10", Name: "APP.mp4", Size: 3 << 20, SHA1: "auxiliary"},
		{ID: "nfo", ParentID: "10", Name: "movie.nfo", PickCode: "nfo", SHA1: "nfo"},
		{ID: "poster", ParentID: "10", Name: "poster.jpg", SHA1: "poster"},
		{ID: "fanart", ParentID: "10", Name: "fanart.jpg", SHA1: "fanart"},
	}
	client.list = func(context.Context, string, string, int, int) (pan.FilePage, error) {
		return pan.FilePage{Files: entries, Total: len(entries), Path: []pan.Directory{{ID: "10"}}}, nil
	}
	metadata := &scanMetadataClient{panClient: client, bodies: map[string][]byte{"nfo": body}}
	library.drive.client = metadata
	queued := library.database.Task.Query().Where(task.TypeEQ("scan")).OnlyX(ctx)
	if err := library.Scan(ctx, TaskJob{ID: queued.ID, Payload: queued.Payload}); err != nil {
		t.Fatal(err)
	}
	film := library.database.Movie.Query().OnlyX(ctx)
	if metadata.reads != 1 || film.Code != "ABP-001" ||
		library.database.File.Query().Where(file.FileIDEQ("auxiliary")).OnlyX(ctx).MovieID != nil {
		t.Fatal("NFO inference did not distinguish the feature from its auxiliary file")
	}
	scrape := NewScrapeService(library, nil, library.images)
	directories, err := scrape.directories(ctx, metadataPayload{Source: source, MovieID: film.ID}, library.drive.snapshot().authorizationVersion)
	if err != nil || len(directories) != 1 || directories[0].Shared {
		t.Fatalf("small video blocked reuse of the movie NFO: %+v err=%v", directories, err)
	}
	live, found := findDirectoryNFO(film.Code, directories[0])
	observed := make(scanObservations)
	observed.add("10", entries)
	compact, compactFound := findNFO(film.Code, false, observed["10"], func(entry observedFile) string { return entry.Name })
	if !found || !compactFound || live.Name != "movie.nfo" || compact.Name != live.Name {
		t.Fatal("live metadata reads and scan observations selected different NFOs")
	}
	snapshot := metadataSnapshot{
		Videos:      videoFingerprint([]pan.File{entries[0]}),
		Directories: []metadataDirectorySnapshot{directorySnapshot("10", entries[2], entries[3], entries[4])},
	}
	indexed := library.database.Movie.Query().Where(movie.IDEQ(film.ID)).WithFiles().OnlyX(ctx)
	if !snapshot.matches(indexed, observed) {
		t.Fatal("an auxiliary video invalidated an unchanged NFO snapshot")
	}
}

func TestFixedVideoSizeThresholdAppliesToFilenameAndOfflineIdentity(t *testing.T) {
	library, _, payload := libraryFixture(t)
	entries := []pan.File{
		{ID: "negative", Name: "ABP-001.mp4", Size: -1},
		{ID: "unknown-size", Name: "ABP-001.mp4"},
		{ID: "small", Name: "ABP-001.mp4", Size: (100 << 20) - 1},
		{ID: "boundary", Name: "ABP-001.mp4", Size: 100 << 20},
		{ID: "large", Name: "ABP-001.mkv", Size: 1 << 30},
	}
	for _, offline := range []bool{false, true} {
		if offline {
			payload.OfflineTaskID, payload.TargetID, payload.Code, payload.JavDBID = 1, "folder", "ABP-001", "catalogue"
		}
		videos, err := identifyScanVideosForTest(t.Context(), library, payload, entries)
		if err != nil || videos["negative"].Code != "" || videos["unknown-size"].Code != "" ||
			videos["small"].Code != "" || videos["boundary"].Code != "ABP-001" || videos["large"].Code != "ABP-001" {
			t.Fatalf("fixed size threshold changed for offline=%t: %+v err=%v", offline, videos, err)
		}
	}
}
