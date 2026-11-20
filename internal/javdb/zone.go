package javdb

import "github.com/ppxb/miyabi/internal/domain"

var zoneCodes = map[domain.Zone]int{
	domain.ZoneCensored:   0,
	domain.ZoneUncensored: 1,
	domain.ZoneWestern:    2,
	domain.ZoneFC2:        3,
	domain.ZoneAnime:      4,
}
