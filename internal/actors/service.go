// Package actors exposes the canonical actor read model: provider-scoped Actor
// rows are linked to one ActorProfile when they are written, and the profile is
// enriched with provider biographies and gfriends images on its first view.
package actors

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/ppxb/miyabi/internal/atomicfile"
	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/actor"
	"github.com/ppxb/miyabi/internal/ent/actoralias"
	"github.com/ppxb/miyabi/internal/ent/actorimage"
	"github.com/ppxb/miyabi/internal/ent/actorprofile"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/predicate"
	"github.com/ppxb/miyabi/internal/gfriends"
	"golang.org/x/sync/singleflight"
)

// ErrNotFound is returned when no actor matches the requested key.
var ErrNotFound = errors.New("actor not found")

// Source is one provider-scoped reference to the canonical actor.
type Source struct {
	Provider string `json:"provider"`
	SourceID string `json:"source_id"`
	Name     string `json:"name"`
	Avatar   string `json:"avatar,omitempty"`
}

// Summary is a catalog-row view of one canonical actor.
type Summary struct {
	Key        string   `json:"key"`
	Name       string   `json:"name"`
	NameZHT    string   `json:"name_zht,omitempty"`
	Avatar     string   `json:"avatar,omitempty"`
	MovieCount int      `json:"movie_count"`
	Sources    []Source `json:"sources"`
}

// Detail adds aliases, biography fields and the image count for the actor page.
type Detail struct {
	Summary
	Aliases      []string `json:"aliases"`
	Names        []string `json:"names"`
	ImageCount   int      `json:"image_count"`
	SummaryText  string   `json:"summary,omitempty"`
	Birthday     string   `json:"birthday,omitempty"`
	Height       int      `json:"height,omitempty"`
	CupSize      string   `json:"cup_size,omitempty"`
	Measurements string   `json:"measurements,omitempty"`
	BloodType    string   `json:"blood_type,omitempty"`
	Nationality  string   `json:"nationality,omitempty"`
	Hobby        string   `json:"hobby,omitempty"`
	Skill        string   `json:"skill,omitempty"`
	DebutDate    string   `json:"debut_date,omitempty"`
	SourceURL    string   `json:"source_url,omitempty"`
}

// Page is one page of the actor catalog.
type Page struct {
	Actors  []Summary `json:"actors"`
	Total   int       `json:"total"`
	Page    int       `json:"page"`
	HasMore bool      `json:"has_more"`
}

// Service builds canonical actor views from the ent actor rows and gfriends.
type Service struct {
	db        *ent.Client
	gfriends  *gfriends.Client
	dataDir   string
	imageHTTP *http.Client
	providers []Provider
	persistMu sync.Mutex
	// enriching shares one provider lookup between concurrent first views.
	enriching singleflight.Group
}

func New(db *ent.Client, g *gfriends.Client, dataDir string, imageHTTP *http.Client, providers ...Provider) *Service {
	return &Service{db: db, gfriends: g, dataDir: dataDir, imageHTTP: imageHTTP, providers: providers}
}

const actorSourcesSetting = "metadata.actor_sources"

var actorSourceIDs = map[string]bool{"av-league": true, "gfriends": true}

func defaultActorSources() []domain.SourceToggle {
	return []domain.SourceToggle{{ID: "av-league", Enabled: true}, {ID: "gfriends", Enabled: true}}
}

// Settings returns the actor data source switches (merged with defaults).
func (s *Service) Settings(ctx context.Context) ([]domain.SourceToggle, error) {
	stored, found, err := database.LoadSetting[[]domain.SourceToggle](ctx, s.db, actorSourcesSetting)
	if err != nil {
		return nil, err
	}
	if !found {
		return defaultActorSources(), nil
	}
	return domain.MergeSourceToggles(stored, defaultActorSources()), nil
}

// UpdateSettings validates and persists the actor data source switches.
func (s *Service) UpdateSettings(ctx context.Context, settings []domain.SourceToggle) error {
	if err := domain.ValidateSourceToggles(settings, actorSourceIDs, "演员来源"); err != nil {
		return err
	}
	return database.SaveSetting(ctx, s.db, actorSourcesSetting, settings)
}

func (s *Service) sourceEnabled(ctx context.Context, id string) bool {
	settings, err := s.Settings(ctx)
	if err != nil {
		return true
	}
	for _, item := range settings {
		if item.ID == id {
			return item.Enabled
		}
	}
	return true
}

// normalizeName lowercases and drops all whitespace so "... りり" == "...りり".
func normalizeName(name string) string {
	return domain.NormalizeActorName(name)
}

// Catalog lists canonical actors that appear in at least one movie, optionally
// filtered by a normalized substring of any of their names. Filtering, ordering
// and paging run in SQL on ActorProfile.
func (s *Service) Catalog(ctx context.Context, query string, page, limit int) (Page, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	where := []predicate.ActorProfile{actorprofile.HasSourcesWith(actor.HasMovies())}
	if q := normalizeName(query); q != "" {
		where = append(where, actorprofile.Or(
			actorprofile.MatchKeyContains(q),
			actorprofile.HasAliasesWith(actoralias.NormalizedContains(q)),
		))
	}
	total, err := s.db.ActorProfile.Query().Where(where...).Count(ctx)
	if err != nil {
		return Page{}, err
	}
	profiles, err := s.db.ActorProfile.Query().Where(where...).
		Order(ent.Asc(actorprofile.FieldName), ent.Asc(actorprofile.FieldID)).
		Offset((page - 1) * limit).Limit(limit).
		WithSources(withSourceMovies).
		All(ctx)
	if err != nil {
		return Page{}, err
	}
	items := make([]Summary, 0, len(profiles))
	for _, p := range profiles {
		items = append(items, summaryOf(p))
	}
	return Page{Actors: items, Total: total, Page: page, HasMore: page*limit < total}, nil
}

func withSourceMovies(q *ent.ActorQuery) {
	q.Order(ent.Asc(actor.FieldProvider), ent.Asc(actor.FieldSourceID)).
		WithMovies(func(q *ent.MovieQuery) { q.Select(movie.FieldID) })
}

// summaryOf builds the catalog view of a profile loaded via withSourceMovies.
func summaryOf(p *ent.ActorProfile) Summary {
	key := strconv.Itoa(p.ID)
	summary := Summary{
		Key:     key,
		Name:    p.Name,
		NameZHT: deref(p.NameZht),
		Avatar:  "/api/actors/" + key + "/avatar",
		Sources: make([]Source, 0, len(p.Edges.Sources)),
	}
	movies := make(map[int]bool)
	for _, r := range p.Edges.Sources {
		if summary.NameZHT == "" {
			summary.NameZHT = deref(r.NameZht)
		}
		summary.Sources = append(summary.Sources, Source{Provider: r.Provider, SourceID: r.SourceID, Name: r.Name, Avatar: deref(r.Avatar)})
		for _, m := range r.Edges.Movies {
			movies[m.ID] = true
		}
	}
	summary.MovieCount = len(movies)
	return summary
}

// sourceNames lists the distinct provider spellings of a loaded profile.
func sourceNames(p *ent.ActorProfile) []string {
	seen := make(map[string]bool, len(p.Edges.Sources))
	for _, r := range p.Edges.Sources {
		seen[r.Name] = true
	}
	return slices.Sorted(maps.Keys(seen))
}

// loadProfile loads a profile that still has provider actors. A profile without
// any would match every movie when its films are listed by name, so it is
// reported as not found.
func (s *Service) loadProfile(ctx context.Context, id int) (*ent.ActorProfile, error) {
	p, err := s.db.ActorProfile.Query().
		Where(actorprofile.ID(id), actorprofile.HasSources()).
		WithSources(withSourceMovies).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	return p, err
}

// Detail returns one canonical actor with aliases, images and biography. The
// gfriends index and the actor providers are only consulted on the first view.
func (s *Service) Detail(ctx context.Context, id int) (Detail, error) {
	p, err := s.loadProfile(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	names := sourceNames(p)
	// Persist gfriends images (rel paths) only when none are stored yet, so a
	// detail request doesn't hit the remote gfriends index on every visit.
	if n, err := s.db.ActorImage.Query().Where(actorimage.HasActorWith(actorprofile.ID(id))).Count(ctx); err == nil && n == 0 {
		if err := s.persistImages(ctx, id, s.imageRels(ctx, names)); err != nil {
			slog.WarnContext(ctx, "persist gfriends actor images failed", "profile", id, "error", err)
		}
	}
	s.enrichIfNeeded(ctx, id, p.Name)

	// Reload: enrichment may have filled the biography.
	bio, err := s.db.ActorProfile.Query().Where(actorprofile.ID(id)).WithAliases().Only(ctx)
	if err != nil {
		return Detail{}, err
	}
	aliases := make(map[string]bool)
	for _, name := range names {
		aliases[name] = true
	}
	for _, alias := range bio.Edges.Aliases {
		aliases[alias.Alias] = true
	}
	delete(aliases, p.Name)
	return Detail{
		Summary:      summaryOf(p),
		Names:        names,
		Aliases:      slices.Sorted(maps.Keys(aliases)),
		ImageCount:   len(s.imageSources(ctx, id, names)),
		SummaryText:  deref(bio.Summary),
		Birthday:     deref(bio.Birthday),
		Height:       derefInt(bio.Height),
		CupSize:      deref(bio.CupSize),
		Measurements: deref(bio.Measurements),
		BloodType:    deref(bio.BloodType),
		Nationality:  deref(bio.Nationality),
		Hobby:        deref(bio.Hobby),
		Skill:        deref(bio.Skill),
		DebutDate:    deref(bio.DebutDate),
		SourceURL:    deref(bio.SourceURL),
	}, nil
}

// Identity is what following an actor needs, read from the database only.
type Identity struct {
	Name   string
	Avatar string
	// Names holds every known spelling, the canonical name first.
	Names []string
	// SourceIDs maps each provider to the actor's id there.
	SourceIDs map[string]string
}

// Identity returns the spellings and provider ids of one canonical actor
// without the network lookups Detail performs on a first view.
func (s *Service) Identity(ctx context.Context, id int) (Identity, error) {
	p, err := s.db.ActorProfile.Query().
		Where(actorprofile.ID(id), actorprofile.HasSources()).
		WithSources().WithAliases().Only(ctx)
	if ent.IsNotFound(err) {
		return Identity{}, ErrNotFound
	}
	if err != nil {
		return Identity{}, err
	}
	identity := Identity{
		Name:      p.Name,
		Avatar:    "/api/actors/" + strconv.Itoa(p.ID) + "/avatar",
		Names:     []string{p.Name},
		SourceIDs: make(map[string]string, len(p.Edges.Sources)),
	}
	for _, source := range p.Edges.Sources {
		if !slices.Contains(identity.Names, source.Name) {
			identity.Names = append(identity.Names, source.Name)
		}
		if identity.SourceIDs[source.Provider] == "" {
			identity.SourceIDs[source.Provider] = source.SourceID
		}
	}
	for _, alias := range p.Edges.Aliases {
		if !slices.Contains(identity.Names, alias.Alias) {
			identity.Names = append(identity.Names, alias.Alias)
		}
	}
	return identity, nil
}

// scriptOf classifies a name as Japanese/CJK, Latin, or other.
func scriptOf(name string) string {
	for _, r := range name {
		if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana) {
			return "ja"
		}
	}
	for _, r := range name {
		if unicode.IsLetter(r) {
			return "latin"
		}
	}
	return "other"
}

type actorSig struct {
	name      string
	script    string
	profileID int
}

// MergeMovieCrossLanguage merges the profiles of the single Japanese-named and
// single English-named actor of one movie when both exist. Cheap, DB-only;
// called right after a movie is scraped so duplicates never surface.
func (s *Service) MergeMovieCrossLanguage(ctx context.Context, movieID int) (int, error) {
	records, err := s.db.Actor.Query().
		Where(actor.HasMoviesWith(movie.IDEQ(movieID))).
		WithProfile(func(q *ent.ActorProfileQuery) { q.Select(actorprofile.FieldID) }).
		All(ctx)
	if err != nil {
		return 0, err
	}
	var ja, en *actorSig
	for _, r := range records {
		if normalizeName(r.Name) == "" || r.Edges.Profile == nil {
			continue
		}
		sig := actorSig{name: r.Name, script: scriptOf(r.Name), profileID: r.Edges.Profile.ID}
		switch sig.script {
		case "ja":
			if ja != nil {
				return 0, nil // ambiguous cast: more than one CJK name
			}
			ja = &sig
		case "latin":
			if en != nil {
				return 0, nil // ambiguous cast: more than one Latin name
			}
			en = &sig
		}
	}
	if ja == nil || en == nil || ja.profileID == en.profileID {
		return 0, nil
	}
	if err := s.mergeProfiles(ctx, ja.profileID, en.profileID); err != nil {
		return 0, err
	}
	return 1, nil
}

// mergeProfiles folds the merged profile into canonical: its actors, aliases
// and images move over, missing biography fields are copied, and the emptied
// profile is deleted so no stale actor page remains.
func (s *Service) mergeProfiles(ctx context.Context, canonical, merged int) error {
	// Serialize with the other actor-image writers. Lock order is persistMu -> db tx
	// everywhere, so taking the mutex before opening the transaction avoids a
	// lock-order inversion (SQLite's BEGIN IMMEDIATE grabs the write lock eagerly).
	s.persistMu.Lock()
	defer s.persistMu.Unlock()

	tx, err := s.db.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	from, err := tx.ActorProfile.Get(ctx, merged)
	if err != nil {
		return err
	}
	if err := tx.Actor.Update().Where(actor.HasProfileWith(actorprofile.ID(merged))).SetProfileID(canonical).Exec(ctx); err != nil {
		return err
	}
	if err := s.fillProfileFrom(ctx, tx, canonical, from); err != nil {
		return err
	}
	if err := s.copyProfileImages(ctx, tx, merged, canonical); err != nil {
		return err
	}
	have, err := tx.ActorAlias.Query().Where(actoralias.HasActorWith(actorprofile.ID(canonical))).Select(actoralias.FieldAlias).Strings(ctx)
	if err != nil {
		return err
	}
	aliases, err := tx.ActorAlias.Query().Where(actoralias.HasActorWith(actorprofile.ID(merged))).All(ctx)
	if err != nil {
		return err
	}
	for _, alias := range aliases {
		if slices.Contains(have, alias.Alias) {
			err = tx.ActorAlias.DeleteOneID(alias.ID).Exec(ctx)
		} else {
			err = tx.ActorAlias.UpdateOneID(alias.ID).SetActorID(canonical).Exec(ctx)
		}
		if err != nil {
			return err
		}
	}
	if _, err := tx.ActorImage.Delete().Where(actorimage.HasActorWith(actorprofile.ID(merged))).Exec(ctx); err != nil {
		return err
	}
	if err := tx.ActorProfile.DeleteOneID(merged).Exec(ctx); err != nil {
		return err
	}
	return tx.Commit()
}

// fillProfileFrom copies non-empty biography fields into pid where pid is empty.
func (s *Service) fillProfileFrom(ctx context.Context, tx *ent.Tx, pid int, from *ent.ActorProfile) error {
	cur, err := tx.ActorProfile.Query().Where(actorprofile.ID(pid)).Only(ctx)
	if err != nil {
		return err
	}
	update := tx.ActorProfile.UpdateOneID(pid)
	if empty(cur.Summary) && !empty(from.Summary) {
		update.SetSummary(*from.Summary)
	}
	if empty(cur.Birthday) && !empty(from.Birthday) {
		update.SetBirthday(*from.Birthday)
	}
	if cur.Height == nil && from.Height != nil {
		update.SetHeight(*from.Height)
	}
	if empty(cur.CupSize) && !empty(from.CupSize) {
		update.SetCupSize(*from.CupSize)
	}
	if empty(cur.Measurements) && !empty(from.Measurements) {
		update.SetMeasurements(*from.Measurements)
	}
	if empty(cur.BloodType) && !empty(from.BloodType) {
		update.SetBloodType(*from.BloodType)
	}
	if empty(cur.Nationality) && !empty(from.Nationality) {
		update.SetNationality(*from.Nationality)
	}
	if empty(cur.Hobby) && !empty(from.Hobby) {
		update.SetHobby(*from.Hobby)
	}
	if empty(cur.Skill) && !empty(from.Skill) {
		update.SetSkill(*from.Skill)
	}
	if empty(cur.DebutDate) && !empty(from.DebutDate) {
		update.SetDebutDate(*from.DebutDate)
	}
	if empty(cur.SourceURL) && !empty(from.SourceURL) {
		update.SetSourceURL(*from.SourceURL)
	}
	if empty(cur.NameZht) && !empty(from.NameZht) {
		update.SetNameZht(*from.NameZht)
	}
	if empty(cur.Avatar) && !empty(from.Avatar) {
		update.SetAvatar(*from.Avatar)
	}
	return update.Exec(ctx)
}

// imageItem is one actor image to append to a profile gallery.
type imageItem struct {
	url      string
	provider string
	role     string
}

// appendImageRows inserts image items into a profile's gallery inside the
// caller's transaction, skipping duplicates by URL and continuing the position
// sequence. Inserts are batched. The caller must hold persistMu.
func (s *Service) appendImageRows(ctx context.Context, tx *ent.Tx, pid int, items []imageItem) error {
	if len(items) == 0 {
		return nil
	}
	existing, err := tx.ActorImage.Query().Where(actorimage.HasActorWith(actorprofile.ID(pid))).All(ctx)
	if err != nil {
		return err
	}
	seen := make(map[string]bool, len(existing))
	position := 0
	for _, im := range existing {
		seen[im.URL] = true
		if im.Position >= position {
			position = im.Position + 1
		}
	}
	builders := make([]*ent.ActorImageCreate, 0, len(items))
	for _, item := range items {
		if item.url == "" || seen[item.url] {
			continue
		}
		seen[item.url] = true
		builders = append(builders, tx.ActorImage.Create().
			SetActorID(pid).SetURL(item.url).SetProvider(item.provider).SetRole(item.role).SetPosition(position))
		position++
	}
	if len(builders) == 0 {
		return nil
	}
	return tx.ActorImage.CreateBulk(builders...).Exec(ctx)
}

// copyProfileImages copies images from one profile to another inside the caller's
// transaction. The caller must hold persistMu.
func (s *Service) copyProfileImages(ctx context.Context, tx *ent.Tx, fromPid, toPid int) error {
	images, err := tx.ActorImage.Query().Where(actorimage.HasActorWith(actorprofile.ID(fromPid))).All(ctx)
	if err != nil {
		return err
	}
	if len(images) == 0 {
		return nil
	}
	items := make([]imageItem, 0, len(images))
	for _, im := range images {
		items = append(items, imageItem{url: im.URL, provider: im.Provider, role: im.Role})
	}
	return s.appendImageRows(ctx, tx, toPid, items)
}

func empty(v *string) bool { return v == nil || *v == "" }

// imageRels merges gfriends image paths across the given names.
func (s *Service) imageRels(ctx context.Context, names []string) []string {
	if !s.sourceEnabled(ctx, "gfriends") {
		return nil
	}
	seen := make(map[string]bool)
	var rels []string
	for _, name := range names {
		images, err := s.gfriends.Images(ctx, name)
		if err != nil {
			continue
		}
		for _, rel := range images {
			if !seen[rel] {
				seen[rel] = true
				rels = append(rels, rel)
			}
		}
	}
	sort.Strings(rels)
	return rels
}

// profileNames lists the distinct provider spellings linked to a profile.
func (s *Service) profileNames(ctx context.Context, pid int) ([]string, error) {
	names, err := s.db.Actor.Query().
		Where(actor.HasProfileWith(actorprofile.ID(pid))).
		Unique(true).
		Select(actor.FieldName).
		Strings(ctx)
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, ErrNotFound
	}
	sort.Strings(names)
	return names, nil
}

// FetchImage returns the actor's index-th image bytes (0-based), loaded from the
// local cache when present and otherwise fetched remotely then cached on disk.
func (s *Service) FetchImage(ctx context.Context, pid, index int) ([]byte, error) {
	names, err := s.profileNames(ctx, pid)
	if err != nil {
		return nil, err
	}
	if index < 0 {
		return nil, ErrNotFound
	}
	sources := s.imageSources(ctx, pid, names)
	if index >= len(sources) {
		return nil, ErrNotFound
	}
	if sources[index].provider == "gfriends" && !s.sourceEnabled(ctx, "gfriends") {
		return nil, ErrNotFound
	}
	return s.loadImage(ctx, sources[index])
}

// FetchAvatar returns the round avatar: the first gfriends image when there is
// one, else the avatar scraped with the actor's movies. av-league photos are
// gallery-only and never used here.
func (s *Service) FetchAvatar(ctx context.Context, pid int) ([]byte, error) {
	names, err := s.profileNames(ctx, pid)
	if err != nil {
		return nil, err
	}
	if s.sourceEnabled(ctx, "gfriends") {
		for _, src := range s.imageSources(ctx, pid, names) {
			if src.provider != "gfriends" {
				continue
			}
			data, err := s.loadImage(ctx, src)
			if err == nil {
				return data, nil
			}
			slog.WarnContext(ctx, "load gfriends actor avatar failed", "profile", pid, "error", err)
			break
		}
	}
	scraped, err := s.db.Actor.Query().
		Where(actor.HasProfileWith(actorprofile.ID(pid)), actor.AvatarNEQ("")).
		Select(actor.FieldAvatar).
		First(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.loadImage(ctx, imageSource{provider: "scraped", ref: *scraped.Avatar})
}

type imageSource struct {
	provider string
	ref      string
}

// imageSources lists the actor's images in display order, preferring persisted
// ActorImage rows and falling back to a live gfriends lookup.
func (s *Service) imageSources(ctx context.Context, pid int, names []string) []imageSource {
	images, err := s.db.ActorImage.Query().
		Where(actorimage.HasActorWith(actorprofile.ID(pid))).
		Order(ent.Asc(actorimage.FieldPosition)).
		All(ctx)
	if err == nil && len(images) > 0 {
		out := make([]imageSource, 0, len(images))
		for _, im := range images {
			out = append(out, imageSource{provider: im.Provider, ref: im.URL})
		}
		return out
	}
	rels := s.imageRels(ctx, names)
	out := make([]imageSource, 0, len(rels))
	for _, rel := range rels {
		out = append(out, imageSource{provider: "gfriends", ref: rel})
	}
	return out
}

func (s *Service) loadImage(ctx context.Context, src imageSource) ([]byte, error) {
	if s.dataDir == "" {
		return s.fetchImageBytes(ctx, src)
	}
	sum := sha1.Sum([]byte(src.provider + "|" + src.ref))
	path := filepath.Join(s.dataDir, "actors", hex.EncodeToString(sum[:]))
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		return data, nil
	}
	data, err := s.fetchImageBytes(ctx, src)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
		_ = atomicfile.WriteFile(path, data, 0o644)
	}
	return data, nil
}

func (s *Service) fetchImageBytes(ctx context.Context, src imageSource) ([]byte, error) {
	if src.provider == "gfriends" {
		return s.gfriends.FetchRel(ctx, src.ref)
	}
	if s.imageHTTP == nil {
		return nil, errors.New("actor image http client not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.ref, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36")
	resp, err := s.imageHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("actor image status " + resp.Status)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(strings.ToLower(ct), "image/") {
		return nil, errors.New("actor image content-type " + ct)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 10<<20))
}

const (
	// enrichTimeout bounds one shared provider lookup.
	enrichTimeout = 20 * time.Second
	// enrichWait is how long a detail request waits for it before answering
	// with the stored profile; the lookup keeps running and a later view sees it.
	enrichWait = 3 * time.Second
)

// enrichIfNeeded consults the actor providers once per profile. enriched_at
// records a finished attempt (found or not), so only transient errors retry on
// a later view; concurrent first views share one attempt. The attempt runs
// detached from the request, so a viewer leaving neither cancels it nor fails
// the other waiters.
func (s *Service) enrichIfNeeded(ctx context.Context, pid int, display string) {
	if len(s.providers) == 0 {
		return
	}
	done := s.enriching.DoChan(strconv.Itoa(pid), func() (any, error) {
		enrichCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), enrichTimeout)
		defer cancel()
		err := s.enrichProfile(enrichCtx, pid, display)
		if err != nil {
			slog.WarnContext(enrichCtx, "enrich actor profile failed", "profile", pid, "error", err)
		}
		return nil, err
	})
	wait := time.NewTimer(enrichWait)
	defer wait.Stop()
	select {
	case <-done:
	case <-wait.C:
	case <-ctx.Done():
	}
}

// enrichProfile fills biography fields (and extra images) from the first
// enabled ActorProvider that knows the actor, then marks the attempt done.
// Provider errors other than not-found are returned and leave it unmarked.
func (s *Service) enrichProfile(ctx context.Context, pid int, display string) error {
	p, err := s.db.ActorProfile.Get(ctx, pid)
	if err != nil {
		return err
	}
	if p.EnrichedAt != nil {
		return nil
	}
	consulted := false
	for _, provider := range s.providers {
		if !s.sourceEnabled(ctx, provider.Name()) {
			continue
		}
		consulted = true
		profile, err := provider.Lookup(ctx, display)
		if errors.Is(err, ErrProviderNotFound) || (err == nil && profile == nil) {
			continue
		}
		if err != nil {
			return err
		}
		if err := s.applyProfile(ctx, pid, provider.Name(), profile); err != nil {
			return err
		}
		break
	}
	// With every provider switched off nothing was tried; a provider enabled
	// later still gets its first attempt.
	if !consulted {
		return nil
	}
	return s.db.ActorProfile.UpdateOneID(pid).SetEnrichedAt(time.Now()).Exec(ctx)
}

// applyProfile writes the provider-sourced biography and images atomically.
// It takes persistMu before the transaction to keep the global lock order
// (persistMu -> db tx) consistent with the other image writers.
func (s *Service) applyProfile(ctx context.Context, pid int, providerName string, p *Profile) error {
	s.persistMu.Lock()
	defer s.persistMu.Unlock()

	tx, err := s.db.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	update := tx.ActorProfile.UpdateOneID(pid)
	if p.Summary != "" {
		update.SetSummary(p.Summary)
	}
	if p.Birthday != "" {
		update.SetBirthday(p.Birthday)
	}
	if p.Height > 0 {
		update.SetHeight(p.Height)
	}
	if p.CupSize != "" {
		update.SetCupSize(p.CupSize)
	}
	if p.Measurements != "" {
		update.SetMeasurements(p.Measurements)
	}
	if p.BloodType != "" {
		update.SetBloodType(p.BloodType)
	}
	if p.Nationality != "" {
		update.SetNationality(p.Nationality)
	}
	if p.Hobby != "" {
		update.SetHobby(p.Hobby)
	}
	if p.Skill != "" {
		update.SetSkill(p.Skill)
	}
	if p.DebutDate != "" {
		update.SetDebutDate(p.DebutDate)
	}
	if p.SourceURL != "" {
		update.SetSourceURL(p.SourceURL)
	}
	if err := update.Exec(ctx); err != nil {
		return err
	}
	if err := s.appendProfileImages(ctx, tx, pid, providerName, p.Images); err != nil {
		return err
	}
	return tx.Commit()
}

// appendProfileImages appends the provider's image URLs inside the caller's
// transaction. The caller must hold persistMu.
func (s *Service) appendProfileImages(ctx context.Context, tx *ent.Tx, pid int, providerName string, urls []string) error {
	items := make([]imageItem, 0, len(urls))
	for _, url := range urls {
		items = append(items, imageItem{url: url, provider: providerName, role: "gallery"})
	}
	return s.appendImageRows(ctx, tx, pid, items)
}

func deref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func derefInt(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

// LinkProfiles attaches the given actors that have no profile yet to the
// canonical ActorProfile of their normalized name, found by match_key or by an
// alias (so a Latin name merged into a CJK profile keeps joining it), and
// creates the profile when none claims the name. Every linked spelling is
// recorded as an alias for catalog search. It only touches the database, so it
// runs in the transaction that writes the actors and none is left unlinked.
func LinkProfiles(ctx context.Context, tx *ent.Tx, actorIDs []int) error {
	if len(actorIDs) == 0 {
		return nil
	}
	rows, err := tx.Actor.Query().
		Where(actor.IDIn(actorIDs...)).
		WithProfile(func(q *ent.ActorProfileQuery) { q.Select(actorprofile.FieldID) }).
		Order(ent.Asc(actor.FieldID)).
		All(ctx)
	if err != nil {
		return err
	}
	// Already linked actors keep their profile; a rename by the upsert only
	// adds the new spelling as an alias so it stays searchable.
	byNorm := make(map[string][]*ent.Actor)
	var linked []*ent.Actor
	for _, r := range rows {
		if r.Edges.Profile != nil {
			linked = append(linked, r)
		} else if norm := normalizeName(r.Name); norm != "" {
			byNorm[norm] = append(byNorm[norm], r)
		}
	}
	norms := slices.Sorted(maps.Keys(byNorm))

	pidByNorm := make(map[string]int, len(norms))
	profiles, err := tx.ActorProfile.Query().Where(actorprofile.MatchKeyIn(norms...)).Select(actorprofile.FieldMatchKey).All(ctx)
	if err != nil {
		return err
	}
	for _, p := range profiles {
		pidByNorm[p.MatchKey] = p.ID
	}
	var unmatched []string
	for _, norm := range norms {
		if pidByNorm[norm] == 0 {
			unmatched = append(unmatched, norm)
		}
	}
	if len(unmatched) > 0 {
		aliases, err := tx.ActorAlias.Query().
			Where(actoralias.NormalizedIn(unmatched...)).
			WithActor(func(q *ent.ActorProfileQuery) { q.Select(actorprofile.FieldID) }).
			All(ctx)
		if err != nil {
			return err
		}
		for _, alias := range aliases {
			if alias.Edges.Actor != nil && pidByNorm[alias.Normalized] == 0 {
				pidByNorm[alias.Normalized] = alias.Edges.Actor.ID
			}
		}
	}

	var creates []*ent.ActorProfileCreate
	var created []string
	for _, norm := range norms {
		if pidByNorm[norm] != 0 {
			continue
		}
		first := byNorm[norm][0]
		creates = append(creates, tx.ActorProfile.Create().
			SetMatchKey(norm).
			SetName(first.Name).
			SetNillableNameZht(nonEmpty(first.NameZht)).
			SetNillableAvatar(nonEmpty(first.Avatar)))
		created = append(created, norm)
	}
	if len(creates) > 0 {
		saved, err := tx.ActorProfile.CreateBulk(creates...).Save(ctx)
		if err != nil {
			return err
		}
		for i, p := range saved {
			pidByNorm[created[i]] = p.ID
		}
	}

	pids := slices.Collect(maps.Values(pidByNorm))
	for _, r := range linked {
		pids = append(pids, r.Edges.Profile.ID)
	}
	if len(pids) == 0 {
		return nil
	}
	existing, err := tx.ActorAlias.Query().
		Where(actoralias.HasActorWith(actorprofile.IDIn(pids...))).
		WithActor(func(q *ent.ActorProfileQuery) { q.Select(actorprofile.FieldID) }).
		All(ctx)
	if err != nil {
		return err
	}
	have := make(map[int]map[string]bool, len(pids))
	for _, pid := range pids {
		have[pid] = make(map[string]bool)
	}
	for _, alias := range existing {
		if alias.Edges.Actor != nil {
			have[alias.Edges.Actor.ID][alias.Alias] = true
		}
	}
	var aliases []*ent.ActorAliasCreate
	for _, r := range linked {
		pid := r.Edges.Profile.ID
		if norm := normalizeName(r.Name); norm != "" && !have[pid][r.Name] {
			have[pid][r.Name] = true
			aliases = append(aliases, tx.ActorAlias.Create().SetActorID(pid).SetAlias(r.Name).SetNormalized(norm))
		}
	}
	for _, norm := range norms {
		pid := pidByNorm[norm]
		ids := make([]int, 0, len(byNorm[norm]))
		for _, r := range byNorm[norm] {
			ids = append(ids, r.ID)
			if !have[pid][r.Name] {
				have[pid][r.Name] = true
				aliases = append(aliases, tx.ActorAlias.Create().SetActorID(pid).SetAlias(r.Name).SetNormalized(norm))
			}
		}
		if err := tx.Actor.Update().Where(actor.IDIn(ids...)).SetProfileID(pid).Exec(ctx); err != nil {
			return err
		}
	}
	if len(aliases) > 0 {
		return tx.ActorAlias.CreateBulk(aliases...).Exec(ctx)
	}
	return nil
}

func nonEmpty(v *string) *string {
	if v == nil || *v == "" {
		return nil
	}
	return v
}

// persistImages seeds the actor's gfriends image paths once, only when the
// profile has no images yet. rels is already deduplicated by imageRels, and a
// single bulk insert is atomic, so a failure never leaves a partial gallery.
func (s *Service) persistImages(ctx context.Context, pid int, rels []string) error {
	if len(rels) == 0 {
		return nil
	}
	s.persistMu.Lock()
	defer s.persistMu.Unlock()

	count, err := s.db.ActorImage.Query().Where(actorimage.HasActorWith(actorprofile.ID(pid))).Count(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	builders := make([]*ent.ActorImageCreate, 0, len(rels))
	for position, rel := range rels {
		builders = append(builders, s.db.ActorImage.Create().
			SetActorID(pid).SetURL(rel).SetProvider("gfriends").SetRole("portrait").SetPosition(position))
	}
	return s.db.ActorImage.CreateBulk(builders...).Exec(ctx)
}
