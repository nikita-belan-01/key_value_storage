package storage

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/nikita-belan-01/key_value_storage/internal/glob"
	"github.com/nikita-belan-01/key_value_storage/pkg/logger"
	"github.com/rs/zerolog"
)

const (
	defaultKeysPerScan = 10
	maxKeysPerScan     = 1000
	cursorPrefix       = "k"
)

var (
	ErrKeyNotExistsOrExpired  = errors.New("key not exist or expired")
	ErrBadCursor              = errors.New("malformed cursor")
	ErrInvalidTTL             = errors.New("invalid ttl")
	ErrInvalidCleanUpInterval = errors.New("invalid clean up interval")
	ErrInvalidMaxKeysValue    = errors.New("invalid max keys value")
)

type Storage struct {
	m    map[string]*entry
	keys keys
	rwMu sync.RWMutex

	head, tail *entry
	maxKeys    int

	stop     chan struct{}
	stopOnce sync.Once

	logger *logger.Logger
}

type entry struct {
	key        string
	value      []byte
	deadline   time.Time
	prev, next *entry
}

func (e *entry) alive(now time.Time) bool {
	return e.deadline.IsZero() || e.deadline.After(now)
}

func New(logger *logger.Logger, cleanUpInterval time.Duration, maxKeys int) (*Storage, error) {
	if cleanUpInterval <= 0 {
		return nil, ErrInvalidCleanUpInterval
	}

	if maxKeys < 0 {
		return nil, ErrInvalidMaxKeysValue
	}

	head, tail := new(entry), new(entry)
	head.next, tail.prev = tail, head

	s := &Storage{
		m:       make(map[string]*entry),
		head:    head,
		tail:    tail,
		maxKeys: maxKeys,
		stop:    make(chan struct{}),
		logger: logger.With(func(c zerolog.Context) zerolog.Context {
			return c.Str("component", "storage")
		}),
	}

	go s.janitor(cleanUpInterval)

	return s, nil
}

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

	if e, ok := s.m[key]; ok {
		e.value, e.deadline = stored, deadline
		s.moveToFront(e)

		return nil
	}

	e := &entry{
		key:      key,
		value:    stored,
		deadline: deadline}
	s.m[key] = e
	s.keys = s.keys.insert(key)
	s.pushFront(e)

	s.evictLocked()

	return nil
}

func (s *Storage) Get(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.rwMu.Lock()
	defer s.rwMu.Unlock()

	e, ok := s.liveLocked(key, time.Now())
	if !ok {
		return nil, ErrKeyNotExistsOrExpired
	}

	s.moveToFront(e)

	out := make([]byte, len(e.value))
	copy(out, e.value)

	return out, nil
}

func (s *Storage) Exists(ctx context.Context, key string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	s.rwMu.RLock()
	defer s.rwMu.RUnlock()

	_, ok := s.liveLocked(key, time.Now())

	return ok, nil
}

func (s *Storage) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.rwMu.Lock()
	defer s.rwMu.Unlock()

	e, ok := s.liveLocked(key, time.Now())
	if !ok {
		return ErrKeyNotExistsOrExpired
	}

	s.unlink(e)
	delete(s.m, key)
	s.keys = s.keys.delete(key)

	return nil
}

func (s *Storage) Keys(ctx context.Context, pattern string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if pattern == "" {
		pattern = "*"
	}

	prefix := glob.LiteralPrefix(pattern)
	now := time.Now()

	s.rwMu.RLock()
	defer s.rwMu.RUnlock()

	var found []string

	for pos := s.keys.scanStart("", false, prefix); pos < len(s.keys); pos++ {
		k := s.keys[pos]

		if prefix != "" && !strings.HasPrefix(k, prefix) {
			break
		}

		if _, ok := s.liveLocked(k, now); !ok {
			continue
		}

		if !glob.Match(pattern, k) {
			continue
		}

		found = append(found, k)
	}

	return found, nil
}

func (s *Storage) Scan(ctx context.Context, cursor string, pattern string, limit int) ([]string, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}

	if pattern == "" {
		pattern = "*"
	}

	after, resuming, err := decodeCursor(cursor)
	if err != nil {
		return nil, "", err
	}

	limit = normalizeLimit(limit)
	prefix := glob.LiteralPrefix(pattern)
	now := time.Now()

	s.rwMu.RLock()
	defer s.rwMu.RUnlock()

	page := make([]string, 0, limit)

	for pos := s.keys.scanStart(after, resuming, prefix); pos < len(s.keys) && len(page) < limit; pos++ {
		k := s.keys[pos]

		if prefix != "" && !strings.HasPrefix(k, prefix) {
			break
		}

		if _, ok := s.liveLocked(k, now); !ok {
			continue
		}

		if matched := glob.Match(pattern, k); !matched {
			continue
		}

		page = append(page, k)
	}

	if len(page) < limit {
		return page, "", nil
	}

	return page, encodeCursor(page[len(page)-1]), nil
}

func (s *Storage) Stop() {
	s.stopOnce.Do(func() { close(s.stop) })
}

func (s *Storage) liveLocked(key string, now time.Time) (*entry, bool) {
	e, ok := s.m[key]
	if !ok || !e.alive(now) {
		return nil, false
	}

	return e, true
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return defaultKeysPerScan
	}

	if limit > maxKeysPerScan {
		return maxKeysPerScan
	}

	return limit
}
