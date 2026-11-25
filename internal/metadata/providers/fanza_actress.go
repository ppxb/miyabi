package providers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
)

// fanzaWorksLimit is one search page; an actress releases far fewer works
// between two daily checks.
const fanzaWorksLimit = 40

// fanzaCard is the part of a FANZA product a works listing needs.
type fanzaCard struct {
	ID                                 string `json:"id"`
	Code                               string `json:"makerContentId"`
	Title                              string
	DeliveryStartDate, MakerReleasedAt *time.Time
	PackageImage                       struct {
		LargeURL  string `json:"largeUrl"`
		MediumURL string `json:"mediumUrl"`
	}
	Actresses []fanzaEntity
}

// ResolveActress finds the FANZA actress behind the first of names that
// matches one. FANZA appends former names in brackets, as in
// "河北彩花（河北彩伽）", and either spelling matches. It returns "" when no
// actress matches and an error when several share the name.
func (s *FANZA) ResolveActress(ctx context.Context, names []string) (string, error) {
	for _, name := range names {
		want := domain.NormalizeActorName(name)
		if want == "" {
			continue
		}
		cards, err := s.cards(ctx, "", name)
		if err != nil {
			return "", err
		}
		found := ""
		for _, card := range cards {
			for _, actress := range card.Actresses {
				if actress.ID == found || !fanzaNameMatches(actress.Name, want) {
					continue
				}
				if found != "" {
					return "", domain.E(domain.KindConflict, "FANZA 中有多位演员叫「"+name+"」，无法确定是哪一位", nil)
				}
				found = actress.ID
			}
		}
		if found != "" {
			return found, nil
		}
	}
	return "", nil
}

// ActressWorks lists an actress's newest works by release date, including
// announced ones. Movie IDs are catalogue codes.
func (s *FANZA) ActressWorks(ctx context.Context, actressID string) ([]domain.Movie, error) {
	cards, err := s.cards(ctx, actressID, "")
	if err != nil {
		return nil, err
	}
	movies := make([]domain.Movie, 0, len(cards))
	for _, card := range cards {
		code := codeid.Normalize(card.Code)
		if code == "" {
			continue
		}
		movies = append(movies, domain.Movie{
			ID: code, Code: code, Title: card.Title,
			Cover: card.PackageImage.LargeURL, Thumbnail: card.PackageImage.MediumURL,
			ReleaseDate: fanzaReleaseDate(card.MakerReleasedAt, card.DeliveryStartDate),
		})
	}
	return movies, nil
}

// cards searches FANZA newest first, by actress id or by keyword, and loads
// the listed products in one batched request.
func (s *FANZA) cards(ctx context.Context, actressID, keyword string) ([]fanzaCard, error) {
	declarations, filter := "$keyword: String!", ""
	variables := map[string]any{"keyword": keyword}
	if actressID != "" {
		declarations += ", $actress: ID!"
		filter = "filter: { actressIds: { ids: [{ id: $actress }] } },"
		variables["actress"] = actressID
	}
	query := fmt.Sprintf(`query ActressSearch(%s) {
  legacySearchPPV(limit: %d, offset: 0, sort: RELEASE_DATE, queryWord: $keyword, %s
    includeExplicit: true, excludeUndelivered: false) {
    result { contents { id } }
  }
}`, declarations, fanzaWorksLimit, filter)
	var response struct {
		Search *struct {
			Result *struct{ Contents []struct{ ID string } }
		} `json:"legacySearchPPV"`
	}
	if err := s.graphQL(ctx, query, variables, &response); err != nil {
		return nil, err
	}
	if response.Search == nil || response.Search.Result == nil {
		return nil, fmt.Errorf("FANZA 未返回完整检索结果")
	}
	contents := response.Search.Result.Contents
	if len(contents) == 0 {
		return nil, nil
	}
	var declared, selections []string
	variables = make(map[string]any, len(contents))
	for i, item := range contents {
		declared = append(declared, fmt.Sprintf("$id%d: ID!", i))
		selections = append(selections, fmt.Sprintf(`m%d: ppvContent(id: $id%d) {
    id makerContentId title deliveryStartDate makerReleasedAt
    packageImage { largeUrl mediumUrl } actresses { id name }
  }`, i, i))
		variables[fmt.Sprintf("id%d", i)] = item.ID
	}
	var products map[string]*fanzaCard
	batch := "query ActressWorks(" + strings.Join(declared, ",") + ") {" + strings.Join(selections, " ") + "}"
	if err := s.graphQL(ctx, batch, variables, &products); err != nil {
		return nil, err
	}
	cards := make([]fanzaCard, 0, len(contents))
	for i := range contents {
		// A product withdrawn since the search comes back null.
		if card := products[fmt.Sprintf("m%d", i)]; card != nil {
			cards = append(cards, *card)
		}
	}
	return cards, nil
}

// fanzaNameMatches compares a normalized name with a FANZA actress name and
// the former names it lists in brackets.
func fanzaNameMatches(full, want string) bool {
	full = strings.NewReplacer("（", "(", "）", ")").Replace(full)
	current, former, _ := strings.Cut(full, "(")
	names := append([]string{current}, strings.FieldsFunc(strings.TrimSuffix(former, ")"), func(r rune) bool {
		return r == '、' || r == ','
	})...)
	for _, name := range names {
		if domain.NormalizeActorName(name) == want {
			return true
		}
	}
	return false
}

// fanzaReleaseDate prefers the maker's release date over the delivery start,
// formatted as the JST calendar day.
func fanzaReleaseDate(released, delivered *time.Time) string {
	date := released
	if date == nil {
		date = delivered
	}
	if date == nil {
		return ""
	}
	return date.In(time.FixedZone("JST", 9*60*60)).Format(time.DateOnly)
}
