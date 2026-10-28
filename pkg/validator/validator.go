package validator

import (
	"errors"
	"fmt"
)

var ErrInvalidLen = errors.New("invalid len")

type LenError struct {
	Min, Max, Got int
}

func (e *LenError) Error() string {
	return fmt.Sprintf("length must be between %d and %d characters, got %d", e.Min, e.Max, e.Got)
}

func (e *LenError) Unwrap() error {
	return ErrInvalidLen
}

func ValidateLen(s string, min, max int) error {
	if n := len([]rune(s)); n < min || n > max {
		return &LenError{Min: min, Max: max, Got: n}
	}

	return nil
}
