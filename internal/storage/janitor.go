package storage

import "time"

func (s *Storage) cleanUp() {
	now := time.Now()

	s.rwMu.Lock()
	defer s.rwMu.Unlock()

	alive := make(keys, 0, len(s.keys))
	var expired int

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
		expired++
	}

	s.keys = alive

	if expired > 0 {
		s.logger.Debug().
			Int("expired", expired).
			Int("keys", len(s.m)).
			Msg("janitor sweep")
	}
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
