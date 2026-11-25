package app

import (
	"context"
	"errors"
	"strconv"

	"github.com/ppxb/miyabi/internal/actors"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/monitor"
)

// followedActor lets the monitor read library actors, whose keys are actor
// profile ids, without depending on the actors package.
func followedActor(service *actors.Service) monitor.ActorLookup {
	return func(ctx context.Context, key string) (monitor.ActorIdentity, error) {
		id, err := strconv.Atoi(key)
		if err != nil || id < 1 {
			return monitor.ActorIdentity{}, domain.E(domain.KindInvalid, "演员编号无效", err)
		}
		identity, err := service.Identity(ctx, id)
		if errors.Is(err, actors.ErrNotFound) {
			return monitor.ActorIdentity{}, domain.E(domain.KindNotFound, "演员不存在", err)
		}
		if err != nil {
			return monitor.ActorIdentity{}, err
		}
		return monitor.ActorIdentity{
			Key: strconv.Itoa(id), Name: identity.Name, Avatar: identity.Avatar, Names: identity.Names,
			FanzaID: identity.SourceIDs["fanza"], JavdbID: identity.SourceIDs["javdb"],
		}, nil
	}
}
