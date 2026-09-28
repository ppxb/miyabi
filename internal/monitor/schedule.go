package monitor

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
)

const (
	dateLayout = "2006-01-02"
	// Unreleased movies are checked every three days, released ones daily
	// until they go stale a month after release.
	preReleaseCheckDays = 3
	staleAfterDays      = 30
	// Actor cursors keep the newest IDs and tolerate a month of late indexing
	// so a delayed listing still counts as new while old backfills do not.
	seenMovieLimit        = 300
	backfillToleranceDays = 30
)

// ActorCursor is what an actor subscription remembers between checks: the
// works already seen and the newest release date that has actually shipped.
type ActorCursor struct {
	Initialized       bool     `json:"initialized"`
	LatestReleaseDate string   `json:"latest_release_date"`
	SeenMovieIDs      []string `json:"seen_movie_ids"`
}

func (cursor ActorCursor) isInitialized() bool {
	return cursor.Initialized || len(cursor.SeenMovieIDs) > 0 || cursor.LatestReleaseDate != ""
}

func decodeCursor(raw string) ActorCursor {
	var cursor ActorCursor
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &cursor)
	}
	return cursor
}

func (cursor ActorCursor) encode() string {
	encoded, _ := json.Marshal(cursor)
	return string(encoded)
}

// snapshotCursor takes a works page as the baseline.
func snapshotCursor(movies []domain.Movie, today string) ActorCursor {
	return ActorCursor{Initialized: true}.advance(movies, today)
}

// newWorks returns unseen works that are not older than the watermark minus
// the backfill tolerance.
func (cursor ActorCursor) newWorks(movies []domain.Movie) []domain.Movie {
	seen := make(map[string]bool, len(cursor.SeenMovieIDs))
	for _, id := range cursor.SeenMovieIDs {
		seen[id] = true
	}
	threshold := ""
	if watermark, err := time.Parse(dateLayout, cursor.LatestReleaseDate); err == nil {
		threshold = watermark.AddDate(0, 0, -backfillToleranceDays).Format(dateLayout)
	}
	var result []domain.Movie
	for _, movie := range movies {
		if seen[movie.ID] || (threshold != "" && movie.ReleaseDate != "" && movie.ReleaseDate < threshold) {
			continue
		}
		result = append(result, movie)
	}
	return result
}

// advance marks the page seen, raises the watermark to the newest release
// that is not in the future, and bounds the seen list to the newest IDs.
func (cursor ActorCursor) advance(movies []domain.Movie, today string) ActorCursor {
	cursor.Initialized = true
	seen := make(map[string]bool, len(cursor.SeenMovieIDs))
	for _, id := range cursor.SeenMovieIDs {
		seen[id] = true
	}
	// Pages list newest first; append oldest first so trimming drops old IDs.
	for i := len(movies) - 1; i >= 0; i-- {
		movie := movies[i]
		if !seen[movie.ID] {
			seen[movie.ID] = true
			cursor.SeenMovieIDs = append(cursor.SeenMovieIDs, movie.ID)
		}
		if movie.ReleaseDate != "" && movie.ReleaseDate <= today && movie.ReleaseDate > cursor.LatestReleaseDate {
			cursor.LatestReleaseDate = movie.ReleaseDate
		}
	}
	if len(cursor.SeenMovieIDs) > seenMovieLimit {
		cursor.SeenMovieIDs = cursor.SeenMovieIDs[len(cursor.SeenMovieIDs)-seenMovieLimit:]
	}
	return cursor
}

// nextMovieCheck applies the polling policy at the configured daily time:
// every three days before release but never past the release day, daily for
// thirty days from release, then stale. An unparsable release date counts
// from creation.
func nextMovieCheck(now time.Time, releaseDate string, createdAt time.Time, checkTime string) (time.Time, bool) {
	today := startOfDay(now)
	release, err := time.ParseInLocation(dateLayout, strings.TrimSpace(releaseDate), time.Local)
	if err != nil {
		release = createdAt
	}
	release = startOfDay(release)
	if today.Before(release) {
		next := checkTimeOn(today.AddDate(0, 0, preReleaseCheckDays), checkTime)
		if releaseAt := checkTimeOn(release, checkTime); releaseAt.After(now) && releaseAt.Before(next) {
			next = releaseAt
		}
		return next, false
	}
	if today.After(release.AddDate(0, 0, staleAfterDays)) {
		return time.Time{}, true
	}
	return nextDaily(now, checkTime), false
}

// nextDaily is the next occurrence of the daily check time after now.
func nextDaily(now time.Time, checkTime string) time.Time {
	next := checkTimeOn(now, checkTime)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

func checkTimeOn(day time.Time, checkTime string) time.Time {
	if !checkTimePattern.MatchString(checkTime) {
		checkTime = defaultCheckTime
	}
	var hour, minute int
	_, _ = fmt.Sscanf(checkTime, "%d:%d", &hour, &minute)
	day = day.In(time.Local)
	return time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, time.Local)
}

func startOfDay(value time.Time) time.Time {
	value = value.In(time.Local)
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.Local)
}
