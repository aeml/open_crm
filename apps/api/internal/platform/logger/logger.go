package logger

import (
	"log/slog"
	"os"
	"strings"
)

func New(goEnv string) *slog.Logger {
	if strings.EqualFold(strings.TrimSpace(goEnv), "production") {
		return slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, nil))
}
