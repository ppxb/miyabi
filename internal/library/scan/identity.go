package scan

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/pan"
)

// CanIdentifyVideo reports whether a file meets the requirements for a feature video or STRM.
func CanIdentifyVideo(entry pan.File) bool {
	if entry.IsDirectory {
		return false
	}
	if domain.IsSTRM(entry.Name) {
		return true
	}
	return domain.IsVideo(entry.Name) && entry.Size >= domain.MinVideoSize
}

// IdentifyVideo parses the filename of a single file to extract a catalogue number.
func IdentifyVideo(file pan.File) Video {
	code, _ := codeid.Parse(file.Name)
	return Video{File: file, Code: code}
}

// IdentifyScanVideos assigns catalogue codes to video files based on existing
// associations, offline download metadata, and filename parsing.
func IdentifyScanVideos(payload Payload, videos []Video, previous map[string]*ent.File) {
	for index := range videos {
		video := &videos[index]
		old := previous[video.ID]
		switch {
		case !CanIdentifyVideo(video.File):
			// Auxiliary files cannot inherit an old or downloaded identity.
			video.Code = ""
		case payload.OfflineTaskID != 0 && payload.TargetID != "":
			video.Code = codeid.Normalize(payload.Code)
		case old != nil && old.AccountID == payload.Source.AccountID &&
			old.Name == video.Name && old.Size == video.Size && old.Sha1 == video.SHA1 &&
			old.Edges.Movie != nil && old.Edges.Movie.JavdbID != nil:
			video.Code = old.Edges.Movie.Code
		default:
			if video.Code == "" {
				video.Code, _ = codeid.Parse(video.Name)
			}
		}
	}
}

// ResolveNFOCodes uses matching NFOs only to identify and validate catalogue codes.
// A sole NFO may identify unnamed videos only in an unshared directory.
func ResolveNFOCodes(ctx context.Context, sess drive.Session, sidecars []pan.File, videos []Video) error {
	if len(sidecars) == 0 {
		return nil
	}
	var firstCode string
	shared := false
	for _, video := range videos {
		if !CanIdentifyVideo(video.File) || video.Code == "" {
			continue
		}
		if firstCode == "" {
			firstCode = video.Code
		} else if !codeid.IsEquivalent(firstCode, video.Code) {
			shared = true
		}
	}
	codes := make(map[string]string)
	for i := range videos {
		video := &videos[i]
		if !CanIdentifyVideo(video.File) || (video.Code == "" && (shared || len(sidecars) != 1)) {
			continue
		}
		entry, found := findNFO(video.Code, shared, sidecars)
		if !found {
			continue
		}
		canonical, read := codes[entry.ID]
		if !read {
			doc, err := readNFO(ctx, sess, entry)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				// Unreadable optional metadata must not prevent filename scraping.
				codes[entry.ID] = ""
				continue
			}
			canonical = doc.Code
			codes[entry.ID] = canonical
		}
		if canonical == "" {
			continue
		}
		expected := video.Code
		if expected == "" {
			expected = firstCode
		}
		if expected != "" && !codeid.IsEquivalent(expected, canonical) {
			return domain.E(domain.KindConflict, fmt.Sprintf("NFO %s 的番号 %s 与视频 %s 的番号 %s 不一致", entry.Name, canonical, video.Name, expected), nil)
		}
		video.Code = canonical
	}
	return nil
}

// A targeted download checks only its matching NFO, not other movies' metadata.
func resolveTargetNFO(ctx context.Context, sess drive.Session, videos []Video) error {
	video := videos[0]
	if !CanIdentifyVideo(video.File) {
		return nil
	}
	entries, err := drive.DirectoryEntries(ctx, sess, video.ParentID)
	if err != nil {
		return err
	}
	var sidecars []pan.File
	shared := false
	for _, entry := range entries {
		if !entry.IsDirectory && strings.EqualFold(path.Ext(entry.Name), ".nfo") {
			sidecars = append(sidecars, entry)
		}
		if entry.ID != video.ID && CanIdentifyVideo(entry) {
			code, _ := codeid.Parse(entry.Name)
			shared = shared || code == "" || !codeid.IsEquivalent(video.Code, code)
		}
	}
	entry, found := findNFO(video.Code, shared, sidecars)
	if !found {
		return nil
	}
	return ResolveNFOCodes(ctx, sess, []pan.File{entry}, videos)
}

// HasEligibleVideos returns whether the given videos slice contains any eligible feature video.
func HasEligibleVideos(videos []Video) bool {
	return slices.ContainsFunc(videos, func(v Video) bool {
		return CanIdentifyVideo(v.File)
	})
}
