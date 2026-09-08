package pan

import (
	"errors"
	"fmt"
)

var ErrUnauthorized = errors.New("pan authorization required")

type apiError struct {
	Code    int
	Message string
}

func (err *apiError) Error() string {
	return fmt.Sprintf("115 error %d: %s", err.Code, err.Message)
}

func (err *apiError) Is(target error) bool {
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
