package scan

import (
	"context"
	"slices"
	"strings"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/library/scrape"
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

// ResolveSingleNFO applies single-NFO heuristic and tolerance matching to a directory's videos.
// When an exclusive directory contains exactly one NFO, its canonical code takes precedence
// over equivalent filename candidate codes (e.g. distributor prefixes such as 200GANA vs GANA,
// or pure dates like 060326-001 vs CARIB-060326-001), preventing dirty prefixes from entering the library.
func ResolveSingleNFO(ctx context.Context, sess drive.Session, sidecars []pan.File, videos []Video) error {
	if len(sidecars) != 1 {
		return nil
	}
	nfoFile := sidecars[0]

	nfoFilenameCode, hasNFOFilenameCode := codeid.Parse(nfoFile.Name)

	hasUnidentifiedEligible := false
	hasDiscrepancy := false
	hasEligible := false

	for _, v := range videos {
		if !CanIdentifyVideo(v.File) {
			continue
		}
		hasEligible = true
		if v.Code == "" {
			hasUnidentifiedEligible = true
		} else if hasNFOFilenameCode && !strings.EqualFold(v.Code, nfoFilenameCode) && codeid.IsEquivalent(v.Code, nfoFilenameCode) {
			hasDiscrepancy = true
		}
	}

	if !hasEligible || (!hasUnidentifiedEligible && !hasDiscrepancy) {
		return nil
	}

	canonicalCode := nfoFilenameCode
	if canonicalCode == "" || hasUnidentifiedEligible {
		doc, err := scrape.ReadNFO(ctx, sess, nfoFile)
		if err != nil {
			// A damaged or inaccessible NFO should not abort the entire scan.
			return nil
		}
		if doc.Code != "" {
			canonicalCode = doc.Code
		}
	}
	if canonicalCode == "" {
		return nil
	}

	// Verify no eligible video conflicts with canonicalCode.
	for _, v := range videos {
		if !CanIdentifyVideo(v.File) || v.Code == "" {
			continue
		}
		if !codeid.IsEquivalent(v.Code, canonicalCode) {
			// A conflicting code exists (e.g. multi-movie folder); do not override.
			return nil
		}
	}

	// All candidate codes are equivalent or empty; assign the canonical NFO code to all eligible videos.
	for i := range videos {
		if CanIdentifyVideo(videos[i].File) {
			if videos[i].Code == "" || codeid.IsEquivalent(videos[i].Code, canonicalCode) {
				videos[i].Code = canonicalCode
			}
		}
	}
	return nil
}

// HasEligibleVideos returns whether the given videos slice contains any eligible feature video.
func HasEligibleVideos(videos []Video) bool {
	return slices.ContainsFunc(videos, func(v Video) bool {
		return CanIdentifyVideo(v.File)
	})
}
