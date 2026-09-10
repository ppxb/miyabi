package service

import (
	"context"
	"errors"

	"github.com/ppxb/miyabi/internal/pan"
)

var errPanDirectoryIncomplete = errors.New("115 目录内容在读取期间变化或分页不完整，请重新扫描")

// A visitor returns false when its search is complete. Validate each page
// before visiting it so incomplete listings cannot commit a scan or authorize
// removing offline history whose output might still exist.
func walkFilePages(ctx context.Context, fetch func(int) (pan.FilePage, error), visit func(pan.FilePage) (bool, error)) error {
	total, offset := -1, 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		page, err := fetch(offset)
		if err != nil {
			return err
		}
		if total == -1 {
			total = page.Total
		}
		offset += len(page.Files)
		if page.Total != total || total < 0 || offset > total ||
			page.HasMore && len(page.Files) == 0 || !page.HasMore && offset != total {
			return errPanDirectoryIncomplete
		}
		more, err := visit(page)
		if err != nil {
			return err
		}
		if !more || !page.HasMore {
			return nil
		}
	}
}

// Offline task pages have no directory-style total count. A lookup or sync
// can stop as soon as all wanted tasks have been found.
func walkOfflinePages(ctx context.Context, fetch func(int) (pan.OfflinePage, error), visit func(pan.OfflinePage) (bool, error)) error {
	for page := 1; ; page++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		remote, err := fetch(page)
		if err != nil {
			return err
		}
		more, err := visit(remote)
		if err != nil {
			return err
		}
		if !more || page >= remote.PageCount {
			return nil
		}
	}
}
