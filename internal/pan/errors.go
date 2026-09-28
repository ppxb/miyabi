package pan

import (
	"errors"
	"fmt"

	"github.com/ppxb/miyabi/internal/domain"
)

var (
	ErrUnauthorized         = domain.E(domain.KindUnauthorized, "pan authorization required", nil)
	ErrNotFound             = domain.E(domain.KindNotFound, "115 file not found", nil)
	ErrOfflineExists        = domain.E(domain.KindConflict, "115 offline task already exists", nil)
	ErrTranscodeUnavailable = errors.New("transcoded playback sources unavailable")
	ErrDownloadUnavailable  = errors.New("direct download URL unavailable")
)

type apiError struct {
	Code    int
	Message string
}

func (err *apiError) Error() string {
	return fmt.Sprintf("115 error %d: %s", err.Code, err.Message)
}

func (err *apiError) PublicMessage() string {
	return err.Message
}

func (err *apiError) DomainKind() domain.Kind {
	switch {
	case err.Is(ErrUnauthorized):
		return domain.KindUnauthorized
	case err.Is(ErrNotFound):
		return domain.KindNotFound
	case err.Is(ErrOfflineExists):
		return domain.KindConflict
	default:
		return domain.KindUpstream
	}
}

func (err *apiError) Is(target error) bool {
	if target == ErrNotFound {
		return err.Code == 430004
	}
	if target == ErrOfflineExists {
		return err.Code == 10008
	}
	if target != ErrUnauthorized {
		return false
	}
	switch err.Code {
	case 99, 40140114, 40140115, 40140116, 40140119, 40140120,
		40140123, 40140124, 40140125, 40140126:
		return true
	default:
		return false
	}
}
