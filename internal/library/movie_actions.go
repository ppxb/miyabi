package library

import (
	"context"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
)

func (s *Service) RescrapeMovie(ctx context.Context, id int, code string) (domain.TaskInfo, error) {
	parent, err := s.scanner.RescrapeMovie(ctx, id, code)
	if err != nil {
		return domain.TaskInfo{}, err
	}
	infos, err := s.Workflows(ctx, []*ent.Task{parent})
	if err != nil {
		return domain.TaskInfo{}, err
	}
	return infos[0], nil
}
