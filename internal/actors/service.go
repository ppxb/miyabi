// Package actors exposes a read model that groups provider-scoped actor rows into
// one canonical actor per normalized name, and enriches it with gfriends images.
package actors

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/ppxb/miyabi/internal/atomicfile"
	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/actor"
	"github.com/ppxb/miyabi/internal/ent/actoralias"
	"github.com/ppxb/miyabi/internal/ent/actorimage"
	"github.com/ppxb/miyabi/internal/ent/actorprofile"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/gfriends"
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
	db              *ent.Client
	gfriends        *gfriends.Client
	dataDir         string
	imageHTTP       *http.Client
	providers       []Provider
	persistMu       sync.Mutex
	enrichAttempted sync.Map
	enrichSem       chan struct{}
}

func New(db *ent.Client, g *gfriends.Client, dataDir string, imageHTTP *http.Client, providers ...Provider) *Service {
	return &Service{db: db, gfriends: g, dataDir: dataDir, imageHTTP: imageHTTP, providers: providers, enrichSem: make(chan struct{}, 3)}
}

// SourceSetting is one actor data source switch kept in settings.
type SourceSetting struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

const actorSourcesSetting = "metadata.actor_sources"

var actorSourceOrder = []string{"av-league", "gfriends"}

func defaultActorSources() []SourceSetting {
	return []SourceSetting{{ID: "av-league", Enabled: true}, {ID: "gfriends", Enabled: true}}
}

// Settings returns the actor data source switches (merged with defaults).
func (s *Service) Settings(ctx context.Context) ([]SourceSetting, error) {
	stored, found, err := database.LoadSetting[[]SourceSetting](ctx, s.db, actorSourcesSetting)
	if err != nil {
		return nil, err
	}
	if !found {
		return defaultActorSources(), nil
	}
	byID := make(map[string]SourceSetting, len(stored))
	for _, item := range stored {
		byID[item.ID] = item
	}
	out := make([]SourceSetting, 0, len(actorSourceOrder))
	for _, id := range actorSourceOrder {
		if item, ok := byID[id]; ok {
			out = append(out, item)
		} else {
			out = append(out, SourceSetting{ID: id, Enabled: true})
		}
	}
	return out, nil
}

// UpdateSettings validates and persists the actor data source switches.
func (s *Service) UpdateSettings(ctx context.Context, settings []SourceSetting) error {
	valid := map[string]bool{"av-league": true, "gfriends": true}
	seen := make(map[string]bool, len(settings))
	byID := make(map[string]SourceSetting, len(settings))
	for _, item := range settings {
		if !valid[item.ID] || seen[item.ID] {
			return errors.New("未知或重复的演员来源")
		}
		seen[item.ID] = true
		byID[item.ID] = item
	}
	if len(settings) != len(actorSourceOrder) {
		return errors.New("请提供完整的演员来源列表")
	}
	out := make([]SourceSetting, 0, len(actorSourceOrder))
	for _, id := range actorSourceOrder {
		out = append(out, byID[id])
	}
	return database.SaveSetting(ctx, s.db, actorSourcesSetting, out)
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

// Key returns a stable URL key for a normalized name.
func Key(norm string) string {
	sum := sha1.Sum([]byte(norm))
	return hex.EncodeToString(sum[:])
}

// normalizeName lowercases and drops all whitespace so "... りり" == "...りり".
func normalizeName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, name)
}

type group struct {
	norm    string
	names   map[string]bool
	nameZHT string
	avatar  string
	sources []Source
	movies  map[int]bool
	ids     []int
}

func (g *group) sortedNames() []string {
	names := make([]string, 0, len(g.names))
	for name := range g.names {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (g *group) display() string {
	names := g.sortedNames()
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

func (s *Service) groups(ctx context.Context) ([]*group, error) {
	records, err := s.db.Actor.Query().
		WithMovies(func(q *ent.MovieQuery) { q.Select(movie.FieldID) }).
		All(ctx)
	if err != nil {
		return nil, err
	}
	byNorm := make(map[string]*group, len(records))
	seenSource := make(map[string]bool)
	for _, r := range records {
		norm := normalizeName(r.Name)
		if norm == "" {
			continue
		}
		g := byNorm[norm]
		if g == nil {
			g = &group{norm: norm, names: make(map[string]bool), movies: make(map[int]bool)}
			byNorm[norm] = g
		}
		g.names[r.Name] = true
		g.ids = append(g.ids, r.ID)
		if g.nameZHT == "" && r.NameZht != nil && *r.NameZht != "" {
			g.nameZHT = *r.NameZht
		}
		var avatar string
		if r.Avatar != nil {
			avatar = *r.Avatar
		}
		if g.avatar == "" && avatar != "" {
			g.avatar = avatar
		}
		key := r.Provider + "\x00" + r.SourceID
		if !seenSource[key] {
			seenSource[key] = true
			g.sources = append(g.sources, Source{Provider: r.Provider, SourceID: r.SourceID, Name: r.Name, Avatar: avatar})
		}
		for _, m := range r.Edges.Movies {
			if m != nil {
				g.movies[m.ID] = true
			}
		}
	}
	groups := make([]*group, 0, len(byNorm))
	for _, g := range byNorm {
		sort.SliceStable(g.sources, func(i, j int) bool {
			if g.sources[i].Provider != g.sources[j].Provider {
				return g.sources[i].Provider < g.sources[j].Provider
			}
			return g.sources[i].SourceID < g.sources[j].SourceID
		})
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].display() < groups[j].display() })
	return groups, nil
}

func (g *group) summary() Summary {
	avatar := g.avatar
	if avatar == "" {
		avatar = avatarPath(Key(g.norm))
	}
	return Summary{
		Key:        Key(g.norm),
		Name:       g.display(),
		NameZHT:    g.nameZHT,
		Avatar:     avatar,
		MovieCount: len(g.movies),
		Sources:    g.sources,
	}
}

func avatarPath(key string) string { return "/api/actors/" + key + "/image/0" }

// Catalog lists canonical actors, optionally filtered by a name substring.
func (s *Service) Catalog(ctx context.Context, query string, page, limit int) (Page, error) {
	groups, err := s.groups(ctx)
	if err != nil {
		return Page{}, err
	}
	q := normalizeName(query)
	filtered := make([]*group, 0, len(groups))
	for _, g := range groups {
		if len(g.movies) == 0 {
			continue
		}
		if q != "" && !g.matches(q) {
			continue
		}
		filtered = append(filtered, g)
	}
	total := len(filtered)
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	items := make([]Summary, 0, end-start)
	for _, g := range filtered[start:end] {
		items = append(items, g.summary())
	}
	return Page{Actors: items, Total: total, Page: page, HasMore: end < total}, nil
}

func (g *group) matches(q string) bool {
	if strings.Contains(g.norm, q) {
		return true
	}
	for name := range g.names {
		if strings.Contains(normalizeName(name), q) {
			return true
		}
	}
	return false
}

func (s *Service) find(ctx context.Context, key string) (*group, error) {
	groups, err := s.groups(ctx)
	if err != nil {
		return nil, err
	}
	for _, g := range groups {
		if Key(g.norm) == key {
			return g, nil
		}
	}
	return nil, ErrNotFound
}

// Detail returns one canonical actor with aliases, images and biography.
func (s *Service) Detail(ctx context.Context, key string) (Detail, error) {
	g, err := s.find(ctx, key)
	if err != nil {
		return Detail{}, err
	}
	detail := Detail{Summary: g.summary(), Names: g.sortedNames(), Aliases: []string{}}
	display := g.display()
	for _, name := range detail.Names {
		if name != display {
			detail.Aliases = append(detail.Aliases, name)
		}
	}
	pid, perr := s.ensureProfile(ctx, g.norm, display, g.nameZHT, g.avatar)
	if perr == nil {
		for _, id := range g.ids {
			_ = s.db.Actor.UpdateOneID(id).SetProfileID(pid).Exec(ctx)
		}
	}
	// Persist gfriends images (rel paths) only when none are stored yet, so a
	// detail request doesn't hit the remote gfriends index on every visit.
	persisted := false
	if perr == nil && pid > 0 {
		if n, cerr := s.db.ActorImage.Query().Where(actorimage.HasActorWith(actorprofile.ID(pid))).Count(ctx); cerr == nil && n > 0 {
			persisted = true
		}
	}
	if !persisted {
		s.persistImages(ctx, g.norm, s.imageRels(ctx, detail.Names))
	}
	if perr == nil {
		s.enqueueEnrichment(pid, display)
		if aliases, aerr := s.db.ActorAlias.Query().
			Where(actoralias.HasActorWith(actorprofile.ID(pid))).
			Select(actoralias.FieldAlias).
			All(ctx); aerr == nil && len(aliases) > 0 {
			detail.Aliases = detail.Aliases[:0]
			for _, alias := range aliases {
				if alias.Alias != display {
					detail.Aliases = append(detail.Aliases, alias.Alias)
				}
			}
			sort.Strings(detail.Aliases)
		}
		if p, err := s.db.ActorProfile.Query().Where(actorprofile.ID(pid)).Only(ctx); err == nil {
			detail.SummaryText = deref(p.Summary)
			detail.Birthday = deref(p.Birthday)
			if p.Height != nil {
				detail.Height = *p.Height
			}
			detail.CupSize = deref(p.CupSize)
			detail.Measurements = deref(p.Measurements)
			detail.BloodType = deref(p.BloodType)
			detail.Nationality = deref(p.Nationality)
			detail.Hobby = deref(p.Hobby)
			detail.Skill = deref(p.Skill)
			detail.DebutDate = deref(p.DebutDate)
			detail.SourceURL = deref(p.SourceURL)
		}
	}
	detail.ImageCount = len(s.imageSources(ctx, g.norm, detail.Names))
	return detail, nil
}

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

// findNames resolves a key to the group's names using a lightweight name-only
// query (no movie edges), for image serving.
func (s *Service) findNames(ctx context.Context, key string) ([]string, error) {
	records, err := s.db.Actor.Query().Select(actor.FieldName).All(ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var names []string
	for _, r := range records {
		norm := normalizeName(r.Name)
		if norm == "" || Key(norm) != key || seen[r.Name] {
			continue
		}
		seen[r.Name] = true
		names = append(names, r.Name)
	}
	if len(names) == 0 {
		return nil, ErrNotFound
	}
	sort.Strings(names)
	return names, nil
}

// FetchImage returns the actor's index-th image bytes (0-based), loaded from the
// local cache when present and otherwise fetched remotely then cached on disk.
func (s *Service) FetchImage(ctx context.Context, key string, index int) ([]byte, error) {
	names, err := s.findNames(ctx, key)
	if err != nil {
		return nil, err
	}
	if index < 0 {
		return nil, ErrNotFound
	}
	norm := normalizeName(names[0])
	sources := s.imageSources(ctx, norm, names)
	if index >= len(sources) {
		return nil, ErrNotFound
	}
	if sources[index].provider == "gfriends" && !s.sourceEnabled(ctx, "gfriends") {
		return nil, ErrNotFound
	}
	return s.loadImage(ctx, sources[index])
}

type imageSource struct {
	provider string
	ref      string
}

// imageSources lists the actor's images in display order, preferring persisted
// ActorImage rows and falling back to a live gfriends lookup.
func (s *Service) imageSources(ctx context.Context, norm string, names []string) []imageSource {
	if pid, err := s.db.ActorProfile.Query().Where(actorprofile.MatchKeyEQ(norm)).OnlyID(ctx); err == nil {
		images, ierr := s.db.ActorImage.Query().
			Where(actorimage.HasActorWith(actorprofile.ID(pid))).
			Order(ent.Asc(actorimage.FieldPosition)).
			All(ctx)
		if ierr == nil && len(images) > 0 {
			out := make([]imageSource, 0, len(images))
			for _, im := range images {
				out = append(out, imageSource{provider: im.Provider, ref: im.URL})
			}
			return out
		}
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

// EnqueueEnrichment ensures a canonical profile per name and schedules
// background ActorProvider enrichment. Used right after scraping so new actors
// are enriched up front instead of on first actor-page view.
func (s *Service) EnqueueEnrichment(ctx context.Context, names []string) {
	for _, name := range names {
		name = strings.TrimSpace(name)
		norm := normalizeName(name)
		if norm == "" {
			continue
		}
		pid, err := s.ensureProfile(ctx, norm, name, "", "")
		if err != nil {
			continue
		}
		s.enqueueEnrichment(pid, name)
	}
}

// enqueueEnrichment runs biography enrichment in the background at most once
// per actor per process, so a detail request never blocks on a remote lookup.
func (s *Service) enqueueEnrichment(pid int, display string) {
	if pid == 0 || len(s.providers) == 0 {
		return
	}
	if _, loaded := s.enrichAttempted.LoadOrStore(pid, true); loaded {
		return
	}
	go func() {
		if s.enrichSem != nil {
			s.enrichSem <- struct{}{}
			defer func() { <-s.enrichSem }()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		s.enrichProfile(ctx, pid, display)
	}()
}

// enrichProfile fills biography fields (and extra images) from the configured
// ActorProviders when the canonical profile has no biography yet.
func (s *Service) enrichProfile(ctx context.Context, pid int, display string) {
	if p, err := s.db.ActorProfile.Query().Where(actorprofile.ID(pid)).Only(ctx); err != nil {
		return
	} else if p.Birthday != nil && *p.Birthday != "" {
		return
	}
	for _, provider := range s.providers {
		if !s.sourceEnabled(ctx, provider.Name()) {
			continue
		}
		profile, err := provider.Lookup(ctx, display)
		if err != nil || profile == nil {
			continue
		}
		s.applyProfile(ctx, pid, provider.Name(), profile)
		return
	}
}

func (s *Service) applyProfile(ctx context.Context, pid int, providerName string, p *Profile) {
	update := s.db.ActorProfile.UpdateOneID(pid)
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
	_ = update.Exec(ctx)
	s.appendProfileImages(ctx, pid, providerName, p.Images)
}

func (s *Service) appendProfileImages(ctx context.Context, pid int, providerName string, urls []string) {
	if len(urls) == 0 {
		return
	}
	s.persistMu.Lock()
	defer s.persistMu.Unlock()
	existing, err := s.db.ActorImage.Query().Where(actorimage.HasActorWith(actorprofile.ID(pid))).All(ctx)
	if err != nil {
		return
	}
	seen := make(map[string]bool, len(existing))
	position := 0
	for _, im := range existing {
		seen[im.URL] = true
		if im.Position >= position {
			position = im.Position + 1
		}
	}
	for _, url := range urls {
		if url == "" || seen[url] {
			continue
		}
		_, _ = s.db.ActorImage.Create().
			SetActorID(pid).SetURL(url).SetProvider(providerName).SetRole("portrait").SetPosition(position).Save(ctx)
		seen[url] = true
		position++
	}
}

func deref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// EnsureProfiles backfills canonical ActorProfile rows (and Actor.profile links,
// ActorAlias spellings) from the existing provider-scoped Actor rows. It is
// idempotent and safe to run at startup; new actors are picked up on the next run.
func (s *Service) EnsureProfiles(ctx context.Context) error {
	records, err := s.db.Actor.Query().All(ctx)
	if err != nil {
		return err
	}
	var errs []error
	type agg struct {
		names   map[string]bool
		nameZHT string
		avatar  string
		ids     []int
	}
	groups := make(map[string]*agg, len(records))
	for _, a := range records {
		norm := normalizeName(a.Name)
		if norm == "" {
			continue
		}
		g := groups[norm]
		if g == nil {
			g = &agg{names: make(map[string]bool)}
			groups[norm] = g
		}
		g.names[a.Name] = true
		g.ids = append(g.ids, a.ID)
		if g.nameZHT == "" && a.NameZht != nil {
			g.nameZHT = *a.NameZht
		}
		if g.avatar == "" && a.Avatar != nil {
			g.avatar = *a.Avatar
		}
	}
	for norm, g := range groups {
		names := make([]string, 0, len(g.names))
		for name := range g.names {
			names = append(names, name)
		}
		sort.Strings(names)
		if len(names) == 0 {
			continue
		}
		pid, perr := s.ensureProfile(ctx, norm, names[0], g.nameZHT, g.avatar)
		if perr != nil {
			errs = append(errs, perr)
			continue
		}
		for _, id := range g.ids {
			_ = s.db.Actor.UpdateOneID(id).SetProfileID(pid).Exec(ctx)
		}
		_, _ = s.db.ActorAlias.Delete().Where(actoralias.HasActorWith(actorprofile.ID(pid))).Exec(ctx)
		for _, name := range names {
			_, _ = s.db.ActorAlias.Create().
				SetActorID(pid).SetAlias(name).SetNormalized(normalizeName(name)).Save(ctx)
		}
	}
	return errors.Join(errs...)
}

func (s *Service) ensureProfile(ctx context.Context, norm, name, nameZHT, avatar string) (int, error) {
	if id, err := s.db.ActorProfile.Query().Where(actorprofile.MatchKeyEQ(norm)).OnlyID(ctx); err == nil {
		return id, nil
	}
	builder := s.db.ActorProfile.Create().SetMatchKey(norm).SetName(name)
	if nameZHT != "" {
		builder.SetNameZht(nameZHT)
	}
	if avatar != "" {
		builder.SetAvatar(avatar)
	}
	profile, err := builder.Save(ctx)
	if err != nil {
		return 0, err
	}
	return profile.ID, nil
}

// persistImages writes the actor's gfriends image paths to ActorImage rows once.
func (s *Service) persistImages(ctx context.Context, norm string, rels []string) {
	s.persistMu.Lock()
	defer s.persistMu.Unlock()
	pid, err := s.db.ActorProfile.Query().Where(actorprofile.MatchKeyEQ(norm)).OnlyID(ctx)
	if err != nil || len(rels) == 0 {
		return
	}
	if count, cerr := s.db.ActorImage.Query().Where(actorimage.HasActorWith(actorprofile.ID(pid))).Count(ctx); cerr == nil && count > 0 {
		return
	}
	for position, rel := range rels {
		_, _ = s.db.ActorImage.Create().
			SetActorID(pid).SetURL(rel).SetProvider("gfriends").SetRole("portrait").SetPosition(position).Save(ctx)
	}
}
