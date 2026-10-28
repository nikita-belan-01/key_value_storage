package config

import (
	"errors"

	"github.com/nikita-belan-01/key_value_storage/pkg/default_env"
	"github.com/nikita-belan-01/key_value_storage/pkg/logger"
)

var ErrEmptyLogDir = errors.New("empty log dir")

type Logger struct {
	Dir   string
	Level string
}

func newLogger() (Logger, error) {
	dir := default_env.GetString("LOG_DIR", "")
	if dir == "" {
		return Logger{}, ErrEmptyLogDir
	}

	level := default_env.GetString("LOG_LEVEL", "")
	if err := logger.ValidateLogLevel(level); err != nil {
		return Logger{}, err
	}

	return Logger{
		Dir:   dir,
		Level: level,
	}, nil
}
