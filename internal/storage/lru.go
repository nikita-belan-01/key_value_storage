package storage

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
	var evicted int

	for s.maxKeys > 0 && len(s.m) > s.maxKeys {
		victim := s.tail.prev
		if victim == s.head {
			break
		}

		s.unlink(victim)
		delete(s.m, victim.key)
		s.keys = s.keys.delete(victim.key)

		evicted++
	}

	if evicted > 0 {
		s.logger.Debug().
			Int("evicted", evicted).
			Int("keys", len(s.m)).
			Msg("lru eviction")
	}
}
