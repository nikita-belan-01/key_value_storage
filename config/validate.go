package config

import (
	"errors"
	"time"
)

var (
	ErrEmptySecret     = errors.New("empty secret")
	ErrInvalidPort     = errors.New("parameter must be between 1 and 65535")
	ErrInvalidDuration = errors.New("invalid duration")
	ErrInvalidSize     = errors.New("invalid size")
)

func validatePort(port int) error {
	if port < 1 || port > 65535 {
		return ErrInvalidPort
	}

	return nil
}

func validateDuration(duration time.Duration) error {
	if duration <= 0 {
		return ErrInvalidDuration
	}

	return nil
}

/*
func validateSize[T int | int64](size T) error {
	if size <= 0 {
		return ErrInvalidSize
	}

	return nil
}
*/
