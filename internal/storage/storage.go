package storage

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/nikita-belan-01/key_value_storage/internal/glob"
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

func New(cleanUpInterval time.Duration, maxKeys int) (*Storage, error) {
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

	e, ok := s.m[key]
	if !ok || !e.alive(time.Now()) {
		return nil, ErrKeyNotExistsOrExpired
	}

	s.moveToFront(e)

	out := make([]byte, len(e.value))
	copy(out, e.value)

	return out, nil
}

func (s *Storage) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.rwMu.Lock()
	defer s.rwMu.Unlock()

	e, ok := s.m[key]
	if !ok || !e.alive(time.Now()) {
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

		v, ok := s.m[k]
		if !ok || !v.alive(now) {
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

		v, ok := s.m[k]
		if !ok || !v.alive(now) {
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

func (s *Storage) unlink(e *entry) {
	e.prev.next = e.next
	e.next.prev = e.prev
	e.prev, e.next = nil, nil
}

func (s *Storage) pushFront(e *entry) {
	e.prev, e.next = s.head, s.head.next
	s.head.next.prev = e
	s.head.next = e
}

func (s *Storage) moveToFront(e *entry) {
	if s.head.next == e {
		return
	}

	s.unlink(e)
	s.pushFront(e)
}

func (s *Storage) evictLocked() {
	for s.maxKeys > 0 && len(s.m) > s.maxKeys {
		victim := s.tail.prev
		if victim == s.head {
			return
		}

		s.unlink(victim)
		delete(s.m, victim.key)
		s.keys = s.keys.delete(victim.key)
	}
}

func (s *Storage) cleanUp() {
	now := time.Now()

	s.rwMu.Lock()
	defer s.rwMu.Unlock()

	alive := make(keys, 0, len(s.keys))
	for _, k := range s.keys {
		e, ok := s.m[k]
		if ok && e.alive(now) {
			alive = append(alive, k)
			continue
		}
		if ok {
			s.unlink(e)
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

func (k keys) scanStart(after string, resuming bool, prefix string) int {
	pos := 0

	if resuming {
		p, found := k.binarySearch(after)
		pos = p

		if found {
			pos++
		}
	}

	if prefix != "" {
		if p, _ := k.binarySearch(prefix); p > pos {
			pos = p
		}
	}

	return pos
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

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return defaultKeysPerScan
	}

	if limit > maxKeysPerScan {
		return maxKeysPerScan
	}

	return limit
}

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
