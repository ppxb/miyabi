package scrape

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/nfo"
	"github.com/ppxb/miyabi/internal/pan"
)

// ArtworkOrigin tracks the source sidecars on 115 used to restore artwork.
type ArtworkOrigin struct {
	NFO    pan.File `json:"nfo"`
	Poster pan.File `json:"poster"`
	Fanart pan.File `json:"fanart"`
}

// Keep persisted cover tasks compatible independently of the pan client model.
type artworkFileJSON struct {
	ID          string `json:"id"`
	ParentID    string `json:"parent_id"`
	Name        string `json:"name"`
	IsDirectory bool   `json:"is_directory"`
	Size        int64  `json:"size"`
	PickCode    string `json:"pick_code"`
	SHA1        string `json:"sha1"`
}

type artworkOriginJSON struct {
	NFO    artworkFileJSON `json:"nfo"`
	Poster artworkFileJSON `json:"poster"`
	Fanart artworkFileJSON `json:"fanart"`
}

func (origin ArtworkOrigin) MarshalJSON() ([]byte, error) {
	return json.Marshal(artworkOriginJSON{
		NFO: artworkFileJSON(origin.NFO), Poster: artworkFileJSON(origin.Poster), Fanart: artworkFileJSON(origin.Fanart),
	})
}

func (origin *ArtworkOrigin) UnmarshalJSON(data []byte) error {
	var stored artworkOriginJSON
	if err := json.Unmarshal(data, &stored); err != nil {
		return err
	}
	*origin = ArtworkOrigin{NFO: pan.File(stored.NFO), Poster: pan.File(stored.Poster), Fanart: pan.File(stored.Fanart)}
	return nil
}

// MovieDirectory describes a 115 directory containing movie files and sidecars.
type MovieDirectory struct {
	ID       string
	VideoIDs map[string]bool
	Files    []pan.File
	Shared   bool
}

// FindDirectoryNFO selects an NFO file from a directory matching the catalogue code.
func FindDirectoryNFO(code string, directory MovieDirectory) (pan.File, bool) {
	return FindNFO(code, directory.Shared, directory.Files, func(entry pan.File) string {
		if entry.IsDirectory {
			return ""
		}
		return entry.Name
	})
}

// FindNFO selects a matching NFO according to priority:
// 1. Exact filename match (e.g. STEM.nfo)
// 2. Catalogue code equivalence match (e.g. 200GANA vs GANA)
// 3. Sole NFO in an unshared/private folder
func FindNFO[T any](code string, shared bool, files []T, name func(T) string) (T, bool) {
	wanted := nfo.FileStem(code) + ".nfo"
	var alias, candidate T
	aliasFound, candidates := false, 0
	for _, entry := range files {
		filename := name(entry)
		if strings.EqualFold(filename, wanted) {
			return entry, true
		}
		if !strings.EqualFold(path.Ext(filename), ".nfo") {
			continue
		}
		candidates++
		candidate = entry
		if !aliasFound {
			if candidateCode, ok := codeid.Parse(filename); ok && codeid.IsEquivalent(candidateCode, code) {
				alias, aliasFound = entry, true
			}
		}
	}
	if aliasFound {
		return alias, true
	}
	if !shared && candidates == 1 {
		return candidate, true
	}
	var zero T
	return zero, false
}

// ReadNFO reads and decodes an NFO file from 115 storage.
func ReadNFO(ctx context.Context, sess drive.Session, entry pan.File) (nfo.Movie, error) {
	body, err := sess.Read(ctx, entry.PickCode, 2<<20)
	if err != nil {
		return nfo.Movie{}, fmt.Errorf("read %s: %w", entry.Name, err)
	}
	doc, err := nfo.Decode(body)
	if err != nil {
		return nfo.Movie{}, err
	}
	code := codeid.Normalize(doc.Code)
	if code == "" {
		code, _ = codeid.Parse(entry.Name)
	}
	doc.Code = code
	return doc, nil
}

// DirectoryNFO locates, reads, and validates an NFO file and its referenced poster/fanart in a directory.
func DirectoryNFO(ctx context.Context, sess drive.Session, wantedCode string, directory MovieDirectory) (nfo.Movie, *ArtworkOrigin, bool, error) {
	entry, found := FindDirectoryNFO(wantedCode, directory)
	if !found {
		return nfo.Movie{}, nil, false, nil
	}
	doc, err := ReadNFO(ctx, sess, entry)
	if err != nil {
		return nfo.Movie{}, nil, false, err
	}
	if !codeid.IsEquivalent(doc.Code, wantedCode) {
		return nfo.Movie{}, nil, false, domain.E(domain.KindConflict, fmt.Sprintf("NFO %s 的番号与视频不一致", entry.Name), nil)
	}
	posterName, fanartName := doc.Poster(), doc.Fanart
	if posterName == "" {
		posterName = "poster.jpg"
	}
	if fanartName == "" {
		fanartName = "fanart.jpg"
	}
	poster, posterFound := SidecarByName(directory.Files, posterName)
	fanart, fanartFound := SidecarByName(directory.Files, fanartName)
	if !posterFound || !fanartFound {
		return nfo.Movie{}, nil, false, domain.E(domain.KindNotFound, fmt.Sprintf("NFO %s 对应的海报或封面不存在", entry.Name), nil)
	}
	return doc, &ArtworkOrigin{NFO: entry, Poster: poster, Fanart: fanart}, true, nil
}

// SidecarByName locates a non-directory file by name (case-insensitive).
func SidecarByName(files []pan.File, name string) (pan.File, bool) {
	for _, entry := range files {
		if !entry.IsDirectory && strings.EqualFold(entry.Name, name) {
			return entry, true
		}
	}
	return pan.File{}, false
}
