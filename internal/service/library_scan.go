package service

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strings"

	"entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/nfo"
	"github.com/ppxb/miyabi/internal/pan"
)

type ScanProgress struct {
	Stage                 string `json:"stage"`
	CurrentPath           string `json:"current_path"`
	DirectoriesDiscovered int    `json:"directories_discovered"`
	DirectoriesScanned    int    `json:"directories_scanned"`
	FilesScanned          int    `json:"files_scanned"`
	VideoFiles            int    `json:"video_files"`
	MatchedFiles          int    `json:"matched_files"`
	UnmatchedFiles        int    `json:"unmatched_files"`
	Movies                int    `json:"movies"`
	RemovedFiles          int    `json:"removed_files"`
	RemovedMovies         int    `json:"removed_movies"`
	MetadataTotal         int    `json:"metadata_total"`
	MetadataCompleted     int    `json:"metadata_completed"`
}

type scanPayload struct {
	Source        LibrarySource `json:"source"`
	Scan          ScanProgress  `json:"scan"`
	TargetID      string        `json:"target_id,omitempty"`
	TargetPath    string        `json:"target_path,omitempty"`
	TargetFile    bool          `json:"target_file,omitempty"`
	OfflineTaskID int           `json:"offline_task_id,omitempty"`
	Code          string        `json:"code,omitempty"`
	JavDBID       string        `json:"javdb_id,omitempty"`
}

type scanDirectory struct {
	id   string
	path string
}

type scanVideo struct {
	pan.File
	Code string
}

func identifyVideo(file pan.File) scanVideo {
	code, _ := codeid.Parse(file.Name)
	return scanVideo{File: file, Code: code}
}

func (service *LibraryService) identifyScanVideos(payload scanPayload, videos []scanVideo, previous map[string]*ent.File) {
	for index := range videos {
		video := &videos[index]
		old := previous[video.ID]
		switch {
		case !canIdentifyVideo(video.File):
			// Auxiliary files cannot inherit an old or downloaded identity.
			video.Code = ""
		case payload.OfflineTaskID != 0 && payload.TargetID != "":
			video.Code = codeid.Normalize(payload.Code)
		case old != nil && old.AccountID == payload.Source.AccountID &&
			old.Name == video.Name && old.Size == video.Size && old.Sha1 == video.SHA1 &&
			old.Edges.Movie != nil && old.Edges.Movie.JavdbID != nil:
			video.Code = old.Edges.Movie.Code
		default:
			video.Code, _ = codeid.Parse(video.Name)
		}
	}
}

func (service *LibraryService) StartScan(ctx context.Context) (TaskInfo, error) {
	state, err := service.drive.verifiedSource(ctx)
	if err != nil {
		return TaskInfo{}, err
	}
	if err := service.drive.commit.Lock(ctx); err != nil {
		return TaskInfo{}, err
	}
	defer service.drive.commit.Unlock()
	if err := service.checkScanSource(state.source(), state.authorizationVersion); err != nil {
		return TaskInfo{}, err
	}
	return service.tasks.enqueueScan(ctx, state.source())
}

func (service *LibraryService) Scan(ctx context.Context, job TaskJob) error {
	payload, err := decodeTaskPayload[scanPayload](job.Payload)
	if err != nil {
		return err
	}
	if payload.OfflineTaskID != 0 && (payload.TargetID == "" || payload.JavDBID == "" || payload.Code == "") {
		return fmt.Errorf("离线扫描缺少下载位置或 JavDB 影片信息")
	}
	// Index reconciliation and the next jobs commit together. A restart after
	// that commit only needs to finish this task, not enqueue the jobs again.
	if payload.Scan.Stage == "done" {
		return nil
	}
	state, err := service.drive.verifiedSource(ctx)
	if err != nil {
		return err
	}
	source, version := state.source(), state.authorizationVersion
	if source.AccountID != payload.Source.AccountID || source.Directory.ID != payload.Source.Directory.ID {
		return fmt.Errorf("媒体目录或登录账号已变更，请重新扫描")
	}
	// Each execution gets a fresh marker, including after a server restart. An
	// interrupted attempt must not make unvisited files look present on retry.
	scanID := uuid.NewString()
	payload.Source = source
	payload.Scan = ScanProgress{
		Stage: "scanning", CurrentPath: source.Directory.Path, DirectoriesDiscovered: 1,
	}
	start := scanDirectory{id: source.Directory.ID, path: source.Directory.Path}
	observed := make(scanObservations)
	savePage := func(directoryPath string, videos []scanVideo, prepare func([]scanVideo) []scanVideo) error {
		return service.drive.commitSource(ctx, source, version, func() error {
			return service.processScanPage(ctx, job.ID, scanID, directoryPath, videos, &payload, prepare)
		})
	}
	reconcile := func() error {
		return service.drive.commitSource(ctx, source, version, func() error {
			return service.reconcileScan(ctx, job.ID, scanID, &payload, observed)
		})
	}
	if payload.TargetID != "" {
		info, err := service.sourceInfo(ctx, source, version, payload.TargetID)
		if err != nil {
			return fmt.Errorf("read completed download: %w", err)
		}
		if payload.OfflineTaskID != 0 && info.ID == source.Directory.ID {
			return fmt.Errorf("115 返回的是媒体根目录，无法确定本次下载的影片文件")
		}
		payload.TargetPath = fileInfoPath(info)
		payload.TargetFile = !info.IsDirectory
		if payload.TargetFile {
			if !isVideo(info.Name) {
				return fmt.Errorf("115 下载结果不是视频文件")
			}
			payload.Scan.FilesScanned, payload.Scan.VideoFiles = 1, 1
			if err := savePage(path.Dir(payload.TargetPath), []scanVideo{{File: info.File}}, func(videos []scanVideo) []scanVideo {
				if videos[0].Code != "" {
					payload.Scan.MatchedFiles, payload.Scan.Movies = 1, 1
				} else {
					payload.Scan.UnmatchedFiles = 1
				}
				return videos
			}); err != nil {
				return err
			}
			entries, err := service.directoryEntries(ctx, source, version, info.ParentID)
			if err != nil {
				return err
			}
			observed.add(info.ParentID, entries)
			return reconcile()
		}
		start = scanDirectory{id: info.ID, path: payload.TargetPath}
	}
	directories := []scanDirectory{start}
	seen := map[string]bool{start.id: true}
	codes := make(map[string]bool)
	for next := 0; next < len(directories); next++ {
		directory := directories[next]
		payload.Scan.CurrentPath = directory.path
		if err := service.reportScan(ctx, job.ID, payload); err != nil {
			return err
		}
		var unidentified []scanVideo
		var sidecars []pan.File
		directoryCodes := make(map[string]bool)
		err := walkFilePages(ctx, func(offset int) (pan.FilePage, error) {
			return service.scanPage(ctx, source, version, directory.id, offset)
		}, func(page pan.FilePage) (bool, error) {
			videos := make([]scanVideo, 0, len(page.Files))
			observed.add(directory.id, page.Files)
			for _, entry := range page.Files {
				if seen[entry.ID] {
					return false, fmt.Errorf("扫描期间重复遇到文件或目录 %s，请重新扫描", path.Join(directory.path, entry.Name))
				}
				seen[entry.ID] = true
				if entry.IsDirectory {
					directories = append(directories, scanDirectory{id: entry.ID, path: path.Join(directory.path, entry.Name)})
					payload.Scan.DirectoriesDiscovered++
					continue
				}
				payload.Scan.FilesScanned++
				if strings.EqualFold(path.Ext(entry.Name), ".nfo") {
					sidecars = append(sidecars, entry)
				}
				if !isVideo(entry.Name) {
					continue
				}
				payload.Scan.VideoFiles++
				videos = append(videos, scanVideo{File: entry})
			}
			if !page.HasMore {
				payload.Scan.DirectoriesScanned++
			}
			if err := savePage(directory.path, videos, func(identified []scanVideo) []scanVideo {
				matched := identified[:0]
				for _, video := range identified {
					if video.Code != "" {
						matched = append(matched, video)
						payload.Scan.MatchedFiles++
						codes[video.Code] = true
						directoryCodes[video.Code] = true
					} else {
						payload.Scan.UnmatchedFiles++
						unidentified = append(unidentified, video)
					}
				}
				payload.Scan.Movies = len(codes)
				return matched
			}); err != nil {
				return false, err
			}
			return true, nil
		})
		if err != nil {
			return fmt.Errorf("scan %s: %w", directory.path, err)
		}
		// A single NFO describes a single-movie directory, including videos
		// whose filenames do not contain a recognizable code.
		if len(sidecars) == 1 && len(directoryCodes) <= 1 && slices.ContainsFunc(unidentified, func(video scanVideo) bool {
			return canIdentifyVideo(video.File)
		}) {
			body, err := service.readSidecar(ctx, source, version, sidecars[0], 2<<20)
			if err != nil {
				return fmt.Errorf("read scan NFO: %w", err)
			}
			doc, err := nfo.Decode(body)
			if err != nil {
				return err
			}
			code := codeid.Normalize(doc.Code)
			if code == "" {
				code, _ = codeid.Parse(sidecars[0].Name)
			}
			if code != "" && (len(directoryCodes) == 0 || directoryCodes[code]) {
				matched := 0
				for i := range unidentified {
					if canIdentifyVideo(unidentified[i].File) {
						unidentified[i].Code = code
						matched++
					}
				}
				codes[code] = true
				payload.Scan.Movies = len(codes)
				payload.Scan.MatchedFiles += matched
				payload.Scan.UnmatchedFiles -= matched
			}
		}
		for start := 0; start < len(unidentified); start += 100 {
			if err := savePage(directory.path, unidentified[start:min(start+100, len(unidentified))], nil); err != nil {
				return err
			}
		}
	}
	payload.Scan.Stage = "reconciling"
	payload.Scan.CurrentPath = source.Directory.Path
	if err := service.reportScan(ctx, job.ID, payload); err != nil {
		return err
	}
	return reconcile()
}

func (service *LibraryService) checkScanSource(source LibrarySource, version uint64) error {
	_, err := service.drive.sourceState(source, version)
	return err
}

func (service *LibraryService) scanPage(ctx context.Context, source LibrarySource, version uint64, directoryID string, offset int) (pan.FilePage, error) {
	state, err := service.drive.sourceState(source, version)
	if err != nil {
		return pan.FilePage{}, err
	}
	page, err := withPanSourceToken(ctx, service.drive, state, func(token string) (pan.FilePage, error) {
		return service.drive.client.List(ctx, token, directoryID, offset, 100)
	})
	if err != nil {
		return pan.FilePage{}, err
	}
	if err := service.checkScanSource(source, version); err != nil {
		return pan.FilePage{}, err
	}
	if !slices.ContainsFunc(page.Path, func(directory pan.Directory) bool { return directory.ID == source.Directory.ID }) {
		return pan.FilePage{}, fmt.Errorf("该文件夹已移出媒体目录，请重新扫描")
	}
	return page, nil
}

func isVideo(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".mp4", ".mkv", ".avi", ".mov", ".wmv", ".flv", ".webm", ".m4v", ".ts", ".m2ts", ".mts", ".mpg", ".mpeg", ".vob":
		return true
	default:
		return false
	}
}

const minVideoSize int64 = 100 << 20

func canIdentifyVideo(entry pan.File) bool {
	return !entry.IsDirectory && isVideo(entry.Name) && entry.Size >= minVideoSize
}

func (service *LibraryService) indexScanPage(ctx context.Context, taskID int, scanID, directoryPath string, videos []scanVideo, payload *scanPayload) error {
	return service.processScanPage(ctx, taskID, scanID, directoryPath, videos, payload, nil)
}

// Read identities and write their file associations in the same transaction.
// prepare accounts for identified videos and can defer unknown files until the
// directory's NFO is available; deferred writes take their own fresh snapshot.
func (service *LibraryService) processScanPage(ctx context.Context, taskID int, scanID, directoryPath string, videos []scanVideo, payload *scanPayload, prepare func([]scanVideo) []scanVideo) error {
	indexChanged := false
	offlineChanged := false
	err := ent.WithTx(ctx, service.database, func(tx *ent.Tx) error {
		if len(videos) > 0 {
			ids := make([]string, 0, len(videos))
			for _, video := range videos {
				ids = append(ids, video.ID)
			}
			previous, err := tx.File.Query().Where(file.FileIDIn(ids...)).
				Select(file.FieldID, file.FieldFileID, file.FieldName, file.FieldParentID, file.FieldSize,
					file.FieldSha1, file.FieldPickCode, file.FieldAccountID, file.FieldRootID, file.FieldPath, file.FieldMovieID).
				WithMovie(func(q *ent.MovieQuery) { q.Select(movie.FieldID, movie.FieldCode, movie.FieldJavdbID) }).All(ctx)
			if err != nil {
				return fmt.Errorf("load previous file associations: %w", err)
			}
			previousFiles := make(map[string]*ent.File, len(previous))
			for _, entry := range previous {
				previousFiles[entry.FileID] = entry
			}
			if prepare != nil {
				service.identifyScanVideos(*payload, videos, previousFiles)
				videos = prepare(videos)
			}
			ids = ids[:0]
			codes := make(map[string]int)
			for _, video := range videos {
				ids = append(ids, video.ID)
				if video.Code != "" {
					codes[video.Code] = 0
				}
			}
			for _, entry := range previous {
				if film := entry.Edges.Movie; film != nil {
					if _, needed := codes[film.Code]; needed {
						codes[film.Code] = film.ID
					}
				}
			}
			if len(codes) > 0 && payload.OfflineTaskID != 0 && payload.TargetID != "" {
				id, err := indexDownloadedMovie(ctx, tx, *payload)
				if err != nil {
					return err
				}
				for code := range codes {
					codes[code] = id
				}
			} else if len(codes) > 0 {
				numbers := make([]string, 0, len(codes))
				for code, id := range codes {
					if id == 0 {
						numbers = append(numbers, code)
					}
				}
				if len(numbers) > 0 {
					movies, err := tx.Movie.Query().Where(movie.CodeIn(numbers...)).Select(movie.FieldID, movie.FieldCode).All(ctx)
					if err != nil {
						return fmt.Errorf("load scanned movie IDs: %w", err)
					}
					for _, record := range movies {
						codes[record.Code] = record.ID
					}
				}
				// Existing metadata needs no write during a rescan. The SQLite
				// write transaction also protects the missing-code check.
				var builders []*ent.MovieCreate
				for code, id := range codes {
					if id == 0 {
						builders = append(builders, tx.Movie.Create().SetCode(code))
					}
				}
				if len(builders) > 0 {
					created, err := tx.Movie.CreateBulk(builders...).Save(ctx)
					if err != nil {
						return fmt.Errorf("index scanned movies: %w", err)
					}
					for _, record := range created {
						codes[record.Code] = record.ID
					}
				}
			}
			builders := make([]*ent.FileCreate, 0, len(videos))
			var unchanged []string
			var previousMovies []int
			for _, video := range videos {
				old := previousFiles[video.ID]
				if old != nil && old.Name == video.Name && old.ParentID == video.ParentID &&
					old.Size == video.Size && old.Sha1 == video.SHA1 && old.PickCode == video.PickCode &&
					old.AccountID == payload.Source.AccountID && old.RootID == payload.Source.Directory.ID &&
					old.Path == path.Join(directoryPath, video.Name) && valueOrZero(old.MovieID) == codes[video.Code] {
					unchanged = append(unchanged, video.ID)
					continue
				}
				indexChanged = true
				if old != nil && old.MovieID != nil && *old.MovieID != codes[video.Code] {
					previousMovies = append(previousMovies, *old.MovieID)
				}
				builder := tx.File.Create().SetFileID(video.ID).SetName(video.Name).SetSize(video.Size).
					SetPickCode(video.PickCode).SetSha1(video.SHA1).SetParentID(video.ParentID).
					SetAccountID(payload.Source.AccountID).SetRootID(payload.Source.Directory.ID).
					SetPath(path.Join(directoryPath, video.Name)).SetScanID(scanID)
				if video.Code != "" {
					builder.SetMovieID(codes[video.Code])
				}
				builders = append(builders, builder)
			}
			if len(builders) > 0 {
				if err := tx.File.CreateBulk(builders...).OnConflictColumns(file.FieldFileID).
					UpdateNewValues().UpdateMovieID().Exec(ctx); err != nil {
					return fmt.Errorf("index scanned files: %w", err)
				}
			}
			if len(unchanged) > 0 {
				if err := tx.File.Update().Where(file.FileIDIn(unchanged...)).SetScanID(scanID).Exec(ctx); err != nil {
					return fmt.Errorf("mark unchanged scanned files: %w", err)
				}
			}
			removed, err := removeUnreferencedMovies(ctx, tx, previousMovies)
			if err != nil {
				return err
			}
			payload.Scan.RemovedMovies += removed
			if len(videos) > 0 && payload.OfflineTaskID != 0 {
				// Record each committed page with its download, so playback is
				// available before the remaining scan and metadata work finishes.
				record, err := tx.Task.Get(ctx, payload.OfflineTaskID)
				if err != nil {
					return err
				}
				input, err := decodeTaskPayload[offlinePayload](record.Payload)
				if err != nil {
					return err
				}
				known := make(map[string]bool, len(input.FileIDs))
				for _, id := range input.FileIDs {
					known[id] = true
				}
				for _, id := range ids {
					if !known[id] {
						input.FileIDs = append(input.FileIDs, id)
						known[id], offlineChanged = true, true
					}
				}
				if offlineChanged {
					record.Payload, err = setTaskPayloadField(record.Payload, "file_ids", input.FileIDs)
					if err != nil {
						return err
					}
					if err := tx.Task.UpdateOneID(record.ID).SetPayload(record.Payload).Exec(ctx); err != nil {
						return err
					}
				}
			}
		}
		return saveScanProgress(ctx, tx.Task, taskID, *payload)
	})
	if err != nil {
		return fmt.Errorf("save scan page: %w", err)
	}
	if indexChanged {
		service.tasks.NotifyLibraryChanged()
	} else if offlineChanged {
		service.tasks.NotifyOfflineChanged()
	} else {
		service.tasks.Notify()
	}
	return nil
}

func indexDownloadedMovie(ctx context.Context, tx *ent.Tx, payload scanPayload) (int, error) {
	code := codeid.Normalize(payload.Code)
	// Persist the source ID with the file index, before metadata work starts,
	// so another scan can reuse this association even while scraping is queued.
	if err := tx.Movie.Create().SetCode(code).SetJavdbID(payload.JavDBID).
		OnConflict().Ignore().Exec(ctx); err != nil {
		return 0, fmt.Errorf("index downloaded movie: %w", err)
	}
	record, err := tx.Movie.Query().Where(movie.Or(movie.JavdbIDEQ(payload.JavDBID),
		movie.And(movie.CodeEQ(code), movie.JavdbIDIsNil()))).Only(ctx)
	if err != nil {
		return 0, fmt.Errorf("resolve downloaded movie association: %w", err)
	}
	if record.JavdbID == nil {
		if err := tx.Movie.UpdateOneID(record.ID).SetJavdbID(payload.JavDBID).Exec(ctx); err != nil {
			return 0, fmt.Errorf("save downloaded movie identity: %w", err)
		}
	}
	return record.ID, nil
}

// Reconcile runs only after every directory and page succeeded. Deletions and
// their progress record commit together, and cannot touch another scan root.
func (service *LibraryService) reconcileScan(ctx context.Context, taskID int, scanID string, payload *scanPayload, observed scanObservations) error {
	err := ent.WithTx(ctx, service.database, func(tx *ent.Tx) error {
		stale := file.And(libraryFiles(payload.Source), file.ScanIDNEQ(scanID))
		if payload.TargetID != "" {
			if payload.TargetFile {
				stale = file.And(stale, file.FileIDEQ(payload.TargetID))
			} else {
				prefix := strings.TrimSuffix(payload.TargetPath, "/") + "/"
				// SQLite LIKE treats '_' and '%' as patterns and folds ASCII
				// case. Compare the literal prefix to keep sibling paths intact.
				stale = file.And(stale, func(s *sql.Selector) {
					s.Where(sql.ExprP("substr("+s.C(file.FieldPath)+", 1, length(?)) = ?", prefix, prefix))
				})
			}
		}
		movies, err := tx.File.Query().Where(stale).QueryMovie().IDs(ctx)
		if err != nil {
			return fmt.Errorf("find removed movie files: %w", err)
		}
		payload.Scan.RemovedFiles, err = tx.File.Delete().Where(stale).Exec(ctx)
		if err != nil {
			return fmt.Errorf("remove missing file indexes: %w", err)
		}
		removed, err := removeUnreferencedMovies(ctx, tx, movies)
		if err != nil {
			return err
		}
		payload.Scan.RemovedMovies += removed
		indexed := file.And(libraryFiles(payload.Source), file.ScanIDEQ(scanID))
		if payload.OfflineTaskID != 0 {
			files, err := tx.File.Query().Where(indexed).Select(file.FieldFileID).All(ctx)
			if err != nil {
				return err
			}
			record, err := tx.Task.Get(ctx, payload.OfflineTaskID)
			if err != nil {
				return err
			}
			ids := make([]string, 0, len(files))
			for _, entry := range files {
				ids = append(ids, entry.FileID)
			}
			record.Payload, err = setTaskPayloadField(record.Payload, "file_ids", ids)
			if err != nil {
				return err
			}
			if err := tx.Task.UpdateOneID(record.ID).SetPayload(record.Payload).Exec(ctx); err != nil {
				return err
			}
		}
		moviesToScrape, err := tx.File.Query().Where(indexed).QueryMovie().
			WithFiles(func(q *ent.FileQuery) { q.Where(libraryFiles(payload.Source)) }).All(ctx)
		if err != nil {
			return fmt.Errorf("find scanned metadata jobs: %w", err)
		}
		ids := make([]int, 0, len(moviesToScrape))
		for _, record := range moviesToScrape {
			if record.ScrapeStatus == movie.ScrapeStatusDone {
				ids = append(ids, record.ID)
			}
		}
		snapshots, err := completedMetadataSnapshots(ctx, tx.Client(), payload.Source, ids)
		if err != nil {
			return err
		}
		for _, record := range moviesToScrape {
			if snapshot, found := snapshots[record.ID]; found && record.ScrapeStatus == movie.ScrapeStatusDone && snapshot.matches(record, observed) {
				cached, err := service.images.Exists(movieArtwork(record))
				if err != nil {
					return fmt.Errorf("check cached artwork: %w", err)
				}
				if cached {
					continue
				}
			}
			input := metadataPayload{Source: payload.Source, ScanTaskID: taskID, MovieID: record.ID,
				Code: record.Code, JavDBID: valueOrZero(record.JavdbID)}
			encoded, err := encodeTaskPayload(input)
			if err != nil {
				return err
			}
			if err := tx.Task.Create().SetType("scrape").SetPayload(encoded).Exec(ctx); err != nil {
				return fmt.Errorf("enqueue movie metadata: %w", err)
			}
		}
		payload.Scan.Stage = "done"
		return saveScanProgress(ctx, tx.Task, taskID, *payload)
	})
	if err != nil {
		return fmt.Errorf("reconcile library: %w", err)
	}
	if payload.Scan.RemovedFiles > 0 || payload.Scan.RemovedMovies > 0 {
		service.tasks.NotifyLibraryChanged()
	} else {
		service.tasks.Notify()
	}
	return nil
}

func removeUnreferencedMovies(ctx context.Context, tx *ent.Tx, ids []int) (int, error) {
	removed := 0
	// Bound the number of SQLite parameters when a large directory is removed.
	for start := 0; start < len(ids); start += 500 {
		count, err := tx.Movie.Delete().Where(
			movie.IDIn(ids[start:min(start+500, len(ids))]...), movie.Not(movie.HasFiles()),
		).Exec(ctx)
		if err != nil {
			return 0, fmt.Errorf("remove movies without files: %w", err)
		}
		removed += count
	}
	return removed, nil
}

func (service *LibraryService) reportScan(ctx context.Context, taskID int, payload scanPayload) error {
	if err := saveScanProgress(ctx, service.database.Task, taskID, payload); err != nil {
		return err
	}
	service.tasks.Notify()
	return nil
}

func saveScanProgress(ctx context.Context, tasks *ent.TaskClient, taskID int, payload scanPayload) error {
	encoded, err := encodeTaskPayload(payload)
	if err != nil {
		return err
	}
	if err := tasks.UpdateOneID(taskID).SetPayload(encoded).Exec(ctx); err != nil {
		return fmt.Errorf("save scan progress: %w", err)
	}
	return nil
}
