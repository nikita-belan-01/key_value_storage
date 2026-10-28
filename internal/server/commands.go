package server

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/nikita-belan-01/key_value_storage/internal/protocol"
	"github.com/nikita-belan-01/key_value_storage/internal/storage"
)

const (
	scanMatchOption = "MATCH"
	scanCountOption = "COUNT"
	setEXOption     = "EX"
)

func (s Server) exec(ctx context.Context, w *bufio.Writer, cmd *protocol.Command) error {
	s.logger.Debug().
		Str("cmd", string(cmd.Name)).
		Int("args", len(cmd.Args)).
		Msg("command")

	switch cmd.Name {
	case protocol.PingCmdName:
		return protocol.WriteSimple(w, "PONG")
	case protocol.SetCmdName:
		return s.set(ctx, w, cmd)
	case protocol.GetCmdName:
		return s.get(ctx, w, cmd)
	case protocol.ExistsCmdName:
		return s.exists(ctx, w, cmd)
	case protocol.DelCmdName:
		return s.del(ctx, w, cmd)
	case protocol.KeysCmdName:
		return s.keys(ctx, w, cmd)
	case protocol.ScanCmdName:
		return s.scan(ctx, w, cmd)
	default:
		s.logger.Warn().Str("cmd", string(cmd.Name)).Msg("unknown command")

		return protocol.WriteError(w, fmt.Sprintf("ERR unknown command '%s'", cmd.Name))
	}
}

func (s Server) set(ctx context.Context, w *bufio.Writer, cmd *protocol.Command) error {
	if len(cmd.Args) < 2 {
		return wrongArgs(w, "set")
	}

	ttl := s.conf.DefaultTTL

	if (len(cmd.Args)-2)%2 != 0 {
		return protocol.WriteError(w, "ERR syntax error")
	}

	for i := 2; i < len(cmd.Args); i += 2 {
		switch strings.ToUpper(string(cmd.Args[i])) {
		case setEXOption:
			secs, err := strconv.Atoi(string(cmd.Args[i+1]))
			if err != nil || secs <= 0 {
				return protocol.WriteError(w, "ERR invalid expire time in 'set' command")
			}

			ttl = time.Duration(secs) * time.Second
		default:
			return protocol.WriteError(w, "ERR syntax error")
		}
	}

	if err := s.store.Set(ctx, string(cmd.Args[0]), cmd.Args[1], ttl); err != nil {
		return storeError(w, err)
	}

	return protocol.WriteSimple(w, "OK")
}

func (s Server) get(ctx context.Context, w *bufio.Writer, cmd *protocol.Command) error {
	if len(cmd.Args) != 1 {
		return wrongArgs(w, "get")
	}

	value, err := s.store.Get(ctx, string(cmd.Args[0]))
	if errors.Is(err, storage.ErrKeyNotExistsOrExpired) {
		return protocol.WriteNull(w) // missing key is not an error
	}

	if err != nil {
		return storeError(w, err)
	}

	return protocol.WriteBulk(w, value)
}

func (s Server) exists(ctx context.Context, w *bufio.Writer, cmd *protocol.Command) error {
	if len(cmd.Args) == 0 {
		return wrongArgs(w, "exists")
	}

	count := 0

	for _, arg := range cmd.Args {
		ok, err := s.store.Exists(ctx, string(arg))
		if err != nil {
			return storeError(w, err)
		}

		if ok {
			count++
		}
	}

	return protocol.WriteInt(w, count)
}

func (s Server) del(ctx context.Context, w *bufio.Writer, cmd *protocol.Command) error {
	if len(cmd.Args) == 0 {
		return wrongArgs(w, "del")
	}

	deleted := 0

	for _, arg := range cmd.Args {
		err := s.store.Delete(ctx, string(arg))

		switch {
		case err == nil:
			deleted++
		case errors.Is(err, storage.ErrKeyNotExistsOrExpired):
			// missing key is not an error
		default:
			return storeError(w, err)
		}
	}

	return protocol.WriteInt(w, deleted)
}

func (s Server) keys(ctx context.Context, w *bufio.Writer, cmd *protocol.Command) error {
	if len(cmd.Args) != 1 {
		return wrongArgs(w, "keys")
	}

	found, err := s.store.Keys(ctx, string(cmd.Args[0]))
	if err != nil {
		return storeError(w, err)
	}

	return protocol.WriteStringArray(w, found)
}

func (s Server) scan(ctx context.Context, w *bufio.Writer, cmd *protocol.Command) error {
	if len(cmd.Args) == 0 {
		return wrongArgs(w, "scan")
	}

	cursor := string(cmd.Args[0])
	if cursor == "0" {
		cursor = "" // storage uses an empty cursor to start the scan
	}

	pattern := "*"
	limit := 0

	if (len(cmd.Args)-1)%2 != 0 {
		return protocol.WriteError(w, "ERR syntax error")
	}

	for i := 1; i < len(cmd.Args); i += 2 {
		switch strings.ToUpper(string(cmd.Args[i])) {
		case scanMatchOption:
			pattern = string(cmd.Args[i+1])
		case scanCountOption:
			n, err := strconv.Atoi(string(cmd.Args[i+1]))
			if err != nil || n <= 0 {
				return protocol.WriteError(w, "ERR value is not an integer or out of range")
			}

			limit = n
		default:
			return protocol.WriteError(w, "ERR syntax error")
		}
	}

	page, next, err := s.store.Scan(ctx, cursor, pattern, limit)
	if err != nil {
		return storeError(w, err)
	}

	if next == "" {
		next = "0" // clients expect 0 as the end-of-scan cursor
	}

	if err := protocol.WriteArrayHeader(w, 2); err != nil {
		return err
	}

	if err := protocol.WriteBulk(w, []byte(next)); err != nil {
		return err
	}

	return protocol.WriteStringArray(w, page)
}

func wrongArgs(w *bufio.Writer, name string) error {
	return protocol.WriteError(w, fmt.Sprintf("ERR wrong number of arguments for '%s' command", name))
}

func storeError(w *bufio.Writer, err error) error {
	switch {
	case errors.Is(err, storage.ErrBadCursor):
		return protocol.WriteError(w, "ERR invalid cursor")
	case errors.Is(err, storage.ErrInvalidTTL):
		return protocol.WriteError(w, "ERR invalid expire time")
	case errors.Is(err, context.DeadlineExceeded):
		return protocol.WriteError(w, "ERR command time out")
	default:
		return protocol.WriteError(w, "ERR internal error")
	}
}
