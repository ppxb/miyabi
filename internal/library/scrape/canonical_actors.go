package scrape

import (
	"context"
	"fmt"
	"strconv"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/actor"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/nfo"
)

// canonicalActors returns a copy of actors with one stable identity per
// ActorProfile. The media server sees the canonical name and a stable id; the
// original provider id remains in Miyabi's miyabi_provider/miyabi_sourceid
// fields, and duplicate roles are folded into a single entry.
func canonicalActors(ctx context.Context, db *ent.Client, movieID int, actors []nfo.Actor) ([]nfo.Actor, error) {
	if db == nil || movieID <= 0 || len(actors) == 0 {
		return actors, nil
	}
	records, err := db.Actor.Query().
		Where(actor.HasMoviesWith(movie.IDEQ(movieID))).
		WithProfile(func(query *ent.ActorProfileQuery) {
			query.WithAliases()
		}).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("load actor profiles for export: %w", err)
	}
	type identity struct {
		id      int
		name    string
		nameZHT string
	}
	bySource := make(map[[2]string]identity, len(records))
	byName := make(map[string]identity, len(records))
	for _, record := range records {
		if record.Edges.Profile == nil {
			continue
		}
		profile := record.Edges.Profile
		entry := identity{id: profile.ID, name: profile.Name}
		if profile.NameZht != nil {
			entry.nameZHT = *profile.NameZht
		}
		bySource[[2]string{record.Provider, record.SourceID}] = entry
		if profile.MatchKey != "" {
			byName[profile.MatchKey] = entry
		}
		for _, alias := range profile.Edges.Aliases {
			if alias.Normalized != "" {
				byName[alias.Normalized] = entry
			}
			if normalized := domain.NormalizeActorName(alias.Alias); normalized != "" {
				byName[normalized] = entry
			}
		}
	}

	result := make([]nfo.Actor, 0, len(actors))
	seen := make(map[int]bool, len(actors))
	for _, item := range actors {
		entry, ok := bySource[[2]string{item.Provider, item.ID}]
		if !ok {
			entry, ok = byName[domain.NormalizeActorName(item.Name)]
		}
		if !ok {
			result = append(result, item)
			continue
		}
		if seen[entry.id] {
			continue
		}
		seen[entry.id] = true
		item.MediaServerID = "miyabi-actor-" + strconv.Itoa(entry.id)
		if entry.name != "" {
			item.Name = entry.name
		}
		if entry.nameZHT != "" {
			item.NameZHT = entry.nameZHT
		}
		result = append(result, item)
	}
	return result, nil
}

// canonicalExportDocument returns an export-only copy of doc; the caller's
// document (persisted as movie metadata) is never rewritten.
func canonicalExportDocument(ctx context.Context, db *ent.Client, movieID int, doc nfo.Movie) (nfo.Movie, error) {
	actors, err := canonicalActors(ctx, db, movieID, doc.Actors)
	if err != nil {
		return nfo.Movie{}, err
	}
	doc.Actors = actors
	return doc, nil
}
