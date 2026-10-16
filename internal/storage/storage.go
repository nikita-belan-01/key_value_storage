package storage

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"
)

const maxKeysPerScan = 64
const cursorPrefix = "k"

var (
	ErrKeyNotExistsOrExpired  = errors.New("key not exist or expired")
	ErrBadCursor              = errors.New("malformed cursor")
	ErrInvalidTTL             = errors.New("invalid ttl")
	ErrInvalidCleanUpInterval = errors.New("invalid clean up interval")
)

type Storage struct {
	m    map[string]valueWithDeadline
	keys keys
	rwMu sync.RWMutex

	stop     chan struct{}
	stopOnce sync.Once
}

type valueWithDeadline struct {
	value    []byte
	deadline time.Time
}

func (v valueWithDeadline) alive(now time.Time) bool {
	return v.deadline.IsZero() || v.deadline.After(now)
}

func New(cleanUpInterval time.Duration) (*Storage, error) {
	s := &Storage{
		m:    make(map[string]valueWithDeadline),
		stop: make(chan struct{}),
	}

	if cleanUpInterval <= 0 {
		return nil, ErrInvalidCleanUpInterval
	}

	go s.janitor(cleanUpInterval)

	return s, nil
}

type keys []string

func (s *Storage) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if ttl < 0 {
		return ErrInvalidTTL
	}

	stored := make([]byte, len(value))
	copy(stored, value)

	s.rwMu.Lock()
	defer s.rwMu.Unlock()

	var deadline time.Time
	if ttl > 0 {
		deadline = time.Now().Add(ttl)
	}

	if _, ok := s.m[key]; !ok {
		s.keys = s.keys.insert(key)
	}

	s.m[key] = valueWithDeadline{
		value:    stored,
		deadline: deadline,
	}

	return nil
}

func (s *Storage) Get(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.rwMu.RLock()
	defer s.rwMu.RUnlock()

	v, ok := s.m[key]
	if !ok {
		return nil, ErrKeyNotExistsOrExpired
	}
	if !v.alive(time.Now()) {
		return nil, ErrKeyNotExistsOrExpired
	}

	out := make([]byte, len(v.value))
	copy(out, v.value)

	return out, nil
}

func (s *Storage) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.rwMu.Lock()
	defer s.rwMu.Unlock()

	v, ok := s.m[key]
	if !ok || !v.alive(time.Now()) {
		return ErrKeyNotExistsOrExpired
	}

	delete(s.m, key)
	s.keys = s.keys.delete(key)

	return nil
}

func (s *Storage) Scan(ctx context.Context, cursor string, limit int) ([]string, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}

	if limit <= 0 || limit > maxKeysPerScan {
		limit = maxKeysPerScan
	}

	now := time.Now()

	s.rwMu.RLock()
	defer s.rwMu.RUnlock()

	after, resuming, err := decodeCursor(cursor)
	if err != nil {
		return nil, "", err
	}

	pos := 0
	if resuming {
		p, found := s.keys.binarySearch(after)
		pos = p

		if found {
			pos++
		}
	}

	page := make([]string, 0, limit)
	for ; pos < len(s.keys) && len(page) < limit; pos++ {
		k := s.keys[pos]
		v, ok := s.m[k]
		if !ok || !v.alive(now) {
			continue
		}

		page = append(page, k)
	}

	if pos >= len(s.keys) {
		return page, "", nil
	}

	return page, encodeCursor(page[len(page)-1]), nil
}

func (s *Storage) Stop() {
	s.stopOnce.Do(func() { close(s.stop) })
}

func (k keys) binarySearch(target string) (int, bool) {
	left, right := 0, len(k)

	for left < right {
		mid := left + (right-left)/2

		if k[mid] < target {
			left = mid + 1
		} else {
			right = mid
		}
	}

	return left, left < len(k) && k[left] == target
}

func (k keys) insert(key string) keys {
	pos, _ := k.binarySearch(key)

	k = append(k, "")
	copy(k[pos+1:], k[pos:])
	k[pos] = key

	return k
}

func (k keys) delete(key string) keys {
	pos, found := k.binarySearch(key)
	if !found {
		return k
	}

	return append(k[:pos], k[pos+1:]...)
}

func (s *Storage) cleanUp() {
	now := time.Now()

	s.rwMu.Lock()
	defer s.rwMu.Unlock()

	alive := make(keys, 0, len(s.keys))
	for _, k := range s.keys {
		v, ok := s.m[k]
		if ok && v.alive(now) {
			alive = append(alive, k)
			continue
		}

		delete(s.m, k)
	}

	s.keys = alive
}

func (s *Storage) janitor(every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			s.cleanUp()
		case <-s.stop:
			return
		}
	}
}

func encodeCursor(key string) string {
	return cursorPrefix + hex.EncodeToString([]byte(key))
}

func decodeCursor(cursor string) (after string, resuming bool, err error) {
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
