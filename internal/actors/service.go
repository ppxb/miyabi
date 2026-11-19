// Package actors exposes a read model that groups provider-scoped actor rows into
// one canonical actor per normalized name, and enriches it with gfriends images.
package actors

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"sync"
	"unicode"

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

// Detail adds aliases and the number of gfriends images for the actor page.
type Detail struct {
	Summary
	Aliases    []string `json:"aliases"`
	Names      []string `json:"names"`
	ImageCount int      `json:"image_count"`
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
	persistMu sync.Mutex
}

func New(db *ent.Client, g *gfriends.Client) *Service {
	return &Service{db: db, gfriends: g}
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

// Detail returns one canonical actor with aliases and gfriends image count.
func (s *Service) Detail(ctx context.Context, key string) (Detail, error) {
	g, err := s.find(ctx, key)
	if err != nil {
		return Detail{}, err
	}
	detail := Detail{Summary: g.summary(), Names: g.sortedNames()}
	display := g.display()
	for _, name := range detail.Names {
		if name != display {
			detail.Aliases = append(detail.Aliases, name)
		}
	}
	rels := s.imageRels(ctx, detail.Names)
	detail.ImageCount = len(rels)
	// Lazily persist a profile for actors created after the startup backfill, so
	// their aliases/images are stored when their page is first viewed.
	if pid, perr := s.ensureProfile(ctx, g.norm, g.display(), g.nameZHT, g.avatar); perr == nil {
		for _, id := range g.ids {
			_ = s.db.Actor.UpdateOneID(id).SetProfileID(pid).Exec(ctx)
		}
	}
	s.persistImages(ctx, g.norm, rels)
	// Prefer the persisted canonical alias/image rows when they exist.
	if pid, perr := s.db.ActorProfile.Query().Where(actorprofile.MatchKeyEQ(g.norm)).OnlyID(ctx); perr == nil {
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
		if count, cerr := s.db.ActorImage.Query().Where(actorimage.HasActorWith(actorprofile.ID(pid))).Count(ctx); cerr == nil && count > 0 {
			detail.ImageCount = count
		}
	}
	return detail, nil
}

// imageRels merges gfriends image paths across the given names.
func (s *Service) imageRels(ctx context.Context, names []string) []string {
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

// FetchImage returns the actor's index-th gfriends image bytes (0-based). It
// prefers the persisted ActorImage rows and falls back to a live gfriends lookup.
func (s *Service) FetchImage(ctx context.Context, key string, index int) ([]byte, error) {
	names, err := s.findNames(ctx, key)
	if err != nil {
		return nil, err
	}
	if index < 0 {
		return nil, ErrNotFound
	}
	norm := normalizeName(names[0])
	if pid, perr := s.db.ActorProfile.Query().Where(actorprofile.MatchKeyEQ(norm)).OnlyID(ctx); perr == nil {
		images, ierr := s.db.ActorImage.Query().
			Where(actorimage.HasActorWith(actorprofile.ID(pid)), actorimage.PositionEQ(index)).
			All(ctx)
		if ierr == nil && len(images) > 0 {
			return s.gfriends.FetchRel(ctx, images[0].URL)
		}
	}
	rels := s.imageRels(ctx, names)
	if index >= len(rels) {
		return nil, ErrNotFound
	}
	return s.gfriends.FetchRel(ctx, rels[index])
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
