package storage

import (
	"encoding/hex"
	"strings"
)

func encodeCursor(key string) string {
	return cursorPrefix + hex.EncodeToString([]byte(key))
}

func decodeCursor(cursor string) (string, bool, error) {
	if cursor == "" {
		return "", false, nil
	}

	payload, ok := strings.CutPrefix(cursor, cursorPrefix)
	if !ok {
		return "", false, ErrBadCursor
	}

	raw, err := hex.DecodeString(payload)
	if err != nil {
		return "", false, ErrBadCursor
	}

	return string(raw), true, nil
}
