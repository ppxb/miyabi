package actors

import (
	"context"
	"errors"
)

// ErrProviderNotFound means an actor data source has no record for the name.
var ErrProviderNotFound = errors.New("actor provider not found")

// Profile is the richer actor record an ActorProvider can supply.
type Profile struct {
	Name         string
	NameZHT      string
	Summary      string
	Birthday     string
	Height       int
	CupSize      string
	Measurements string
	BloodType    string
	Nationality  string
	Hobby        string
	Skill        string
	DebutDate    string
	SourceURL    string
	SourceID     string
	Aliases      []string
	Images       []string
}

// Provider is an actor data source keyed by name (metatube ActorProvider-like).
type Provider interface {
	// Name identifies the source, e.g. "av-league".
	Name() string
	// Lookup resolves an actor profile by (display) name; ErrProviderNotFound
	// when the source has no matching actor.
	Lookup(ctx context.Context, name string) (*Profile, error)
}
