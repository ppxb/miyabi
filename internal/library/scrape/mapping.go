package scrape

import (
	"context"
	"fmt"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/actor"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/tag"
	"github.com/ppxb/miyabi/internal/nfo"
)

// DetailNFO converts a catalogue MovieDetail into an NFO Document.
func DetailNFO(detail domain.MovieDetail) nfo.Movie {
	doc := nfo.Movie{
		Title:     detail.Title,
		Code:      detail.Code,
		Premiered: detail.ReleaseDate,
		Runtime:   detail.Duration,
		Rating:    detail.Rating,
		IDs:       []nfo.UniqueID{{Type: "javdb", Default: true, Value: detail.ID}},
	}
	if detail.Director != nil {
		doc.Director = nfo.Entity{ID: detail.Director.ID, Name: detail.Director.Name}
	}
	if detail.Maker != nil {
		doc.Studio = nfo.Entity{ID: detail.Maker.ID, Name: detail.Maker.Name}
	}
	if detail.Series != nil {
		doc.Set = nfo.Series{ID: detail.Series.ID, Name: detail.Series.Name}
	}
	for _, person := range detail.Actors {
		doc.Actors = append(doc.Actors, nfo.Actor{
			ID:      person.ID,
			Name:    person.Name,
			NameZHT: person.NameZHT,
			Gender:  person.Gender,
			Thumb:   person.Avatar,
		})
	}
	for _, item := range detail.Tags {
		doc.Tags = append(doc.Tags, nfo.Tag{
			ID:         item.ID,
			Name:       item.Name,
			NameZHT:    item.NameZHT,
			CategoryID: item.CategoryID,
		})
		doc.Genres = append(doc.Genres, item.Name)
	}
	return doc
}

// MovieNFO builds an NFO Document from an existing indexed ent.Movie record.
func MovieNFO(record *ent.Movie) nfo.Movie {
	doc := nfo.Movie{
		Title:    record.Title,
		Code:     record.Code,
		Runtime:  domain.ValueOrZero(record.Duration),
		Rating:   domain.ValueOrZero(record.Rating),
		Director: nfo.Entity{ID: domain.ValueOrZero(record.DirectorID), Name: domain.ValueOrZero(record.DirectorName)},
		Studio:   nfo.Entity{ID: domain.ValueOrZero(record.MakerID), Name: domain.ValueOrZero(record.MakerName)},
		Set:      nfo.Series{ID: domain.ValueOrZero(record.SeriesID), Name: domain.ValueOrZero(record.SeriesName)},
	}
	if record.JavdbID != nil {
		doc.IDs = []nfo.UniqueID{{Type: "javdb", Default: true, Value: *record.JavdbID}}
	}
	if record.ReleaseDate != nil {
		doc.Premiered = record.ReleaseDate.Format(time.DateOnly)
	}
	for _, person := range record.Edges.Actors {
		doc.Actors = append(doc.Actors, nfo.Actor{
			ID:      person.JavdbID,
			Name:    person.Name,
			NameZHT: domain.ValueOrZero(person.NameZht),
			Gender:  string(person.Gender),
			Thumb:   domain.ValueOrZero(person.Avatar),
		})
	}
	for _, item := range record.Edges.Tags {
		doc.Tags = append(doc.Tags, nfo.Tag{
			ID:         item.JavdbID,
			Name:       item.Name,
			NameZHT:    domain.ValueOrZero(item.NameZht),
			CategoryID: item.CategoryID,
		})
		doc.Genres = append(doc.Genres, item.Name)
	}
	return doc
}

// SaveMovieMetadata updates an ent.Movie record and associates actors and tags from an NFO document.
func SaveMovieMetadata(ctx context.Context, tx *ent.Tx, id int, doc nfo.Movie) error {
	update := tx.Movie.UpdateOneID(id).SetTitle(doc.Title).SetScrapeStatus(movie.ScrapeStatusPending).
		ClearActors().ClearTags().ClearJavdbID().ClearReleaseDate().ClearDuration().ClearRating().
		ClearDirectorID().ClearDirectorName().ClearMakerID().ClearMakerName().ClearSeriesID().ClearSeriesName()
	if doc.Code != "" {
		update.SetCode(doc.Code)
	}
	if doc.JavDBID() != "" {
		update.SetJavdbID(doc.JavDBID())
	}
	if doc.Premiered != "" {
		date, err := time.Parse(time.DateOnly, doc.Premiered)
		if err != nil {
			return fmt.Errorf("parse metadata release date: %w", err)
		}
		update.SetReleaseDate(date)
	}
	if doc.Runtime > 0 {
		update.SetDuration(doc.Runtime)
	}
	if doc.Rating > 0 {
		update.SetRating(doc.Rating)
	}
	if doc.Director.ID != "" {
		update.SetDirectorID(doc.Director.ID)
	}
	if doc.Director.Name != "" {
		update.SetDirectorName(doc.Director.Name)
	}
	if doc.Studio.ID != "" {
		update.SetMakerID(doc.Studio.ID)
	}
	if doc.Studio.Name != "" {
		update.SetMakerName(doc.Studio.Name)
	}
	if doc.Set.ID != "" {
		update.SetSeriesID(doc.Set.ID)
	}
	if doc.Set.Name != "" {
		update.SetSeriesName(doc.Set.Name)
	}
	actors := make([]*ent.ActorCreate, 0, len(doc.Actors))
	actorIDs := make([]string, 0, len(doc.Actors))
	for _, person := range doc.Actors {
		if person.ID == "" {
			continue
		}
		gender := actor.Gender(person.Gender)
		if gender == "" {
			gender = actor.GenderUnknown
		}
		actors = append(actors, tx.Actor.Create().SetJavdbID(person.ID).SetName(person.Name).
			SetNameZht(person.NameZHT).SetGender(gender).SetAvatar(person.Thumb))
		actorIDs = append(actorIDs, person.ID)
	}
	if len(actors) > 0 {
		if err := tx.Actor.CreateBulk(actors...).OnConflictColumns(actor.FieldJavdbID).UpdateNewValues().Exec(ctx); err != nil {
			return err
		}
		ids, err := tx.Actor.Query().Where(actor.JavdbIDIn(actorIDs...)).IDs(ctx)
		if err != nil {
			return err
		}
		update.AddActorIDs(ids...)
	}
	tags := make([]*ent.TagCreate, 0, len(doc.Tags))
	tagIDs := make([]string, 0, len(doc.Tags))
	for _, item := range doc.Tags {
		if item.ID == "" || item.CategoryID == "" {
			continue
		}
		tags = append(tags, tx.Tag.Create().SetJavdbID(item.ID).SetName(item.Name).SetNameZht(item.NameZHT).SetCategoryID(item.CategoryID))
		tagIDs = append(tagIDs, item.ID)
	}
	if len(tags) > 0 {
		if err := tx.Tag.CreateBulk(tags...).OnConflictColumns(tag.FieldJavdbID).UpdateNewValues().Exec(ctx); err != nil {
			return err
		}
		ids, err := tx.Tag.Query().Where(tag.JavdbIDIn(tagIDs...)).IDs(ctx)
		if err != nil {
			return err
		}
		update.AddTagIDs(ids...)
	}
	return update.Exec(ctx)
}
