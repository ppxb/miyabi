package logging

import (
	"fmt"
	"io"
	"log/slog"
)

func New(output io.Writer, levelName string) (*slog.Logger, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(levelName)); err != nil {
		return nil, fmt.Errorf("parse log level %q: %w", levelName, err)
	}

	return slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{
		Level: level,
	})), nil
}
