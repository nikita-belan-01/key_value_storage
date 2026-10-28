package default_env

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type number interface {
	uint | uint8 | uint16 | uint32 | uint64 |
		int | int8 | int16 | int32 | int64 |
		float32 | float64
}

type numberType uint8

const (
	typeUint numberType = iota
	typeInt
	typeFloat
)

func GetNumber[T number](key string, defaultValue T) (T, error) {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue, nil
	}

	var zero T
	var bitSize int
	var t numberType

	switch any(zero).(type) {
	case uint:
		t, bitSize = typeUint, strconv.IntSize
	case uint8:
		t, bitSize = typeUint, 8
	case uint16:
		t, bitSize = typeUint, 16
	case uint32:
		t, bitSize = typeUint, 32
	case uint64:
		t, bitSize = typeUint, 64

	case int:
		t, bitSize = typeInt, strconv.IntSize
	case int8:
		t, bitSize = typeInt, 8
	case int16:
		t, bitSize = typeInt, 16
	case int32:
		t, bitSize = typeInt, 32
	case int64:
		t, bitSize = typeInt, 64

	case float32:
		t, bitSize = typeFloat, 32
	case float64:
		t, bitSize = typeFloat, 64
	}

	var result T
	var err error
	switch t {
	case typeUint:
		var v uint64
		v, err = strconv.ParseUint(value, 10, bitSize)
		result = T(v)

	case typeInt:
		var v int64
		v, err = strconv.ParseInt(value, 10, bitSize)
		result = T(v)

	case typeFloat:
		var v float64
		v, err = strconv.ParseFloat(value, bitSize)
		result = T(v)
	}

	if err != nil {
		return zero, fmt.Errorf("env %s: invalid number %q for type %T: %w", key, value, zero, err)
	}

	return result, nil
}

func GetString(key string, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	return value
}

func GetBool(key string, defaultValue bool) (bool, error) {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue, nil
	}

	val, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("env %s: invalid bool value %q: %w", key, value, err)
	}

	return val, nil
}

func GetDuration(key string, defaultValue time.Duration) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue, nil
	}

	val, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("env %s: invalid duration %q: %w", key, value, err)
	}

	return val, nil
}
