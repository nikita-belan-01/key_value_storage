package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/nikita-belan-01/key_value_storage/config"
	"github.com/nikita-belan-01/key_value_storage/internal/protocol"
	"github.com/nikita-belan-01/key_value_storage/internal/storage"
	"github.com/nikita-belan-01/key_value_storage/pkg/logger"
	"github.com/rs/zerolog"
)

var ErrShutdown = errors.New("shutdown: connections did not finish in time")

const acceptRetryDelay = 100 * time.Millisecond

type Server struct {
	conf   *config.Server
	logger *logger.Logger
	store  *storage.Storage
}

func New(conf *config.Server, logger *logger.Logger, store *storage.Storage) *Server {
	return &Server{
		conf:  conf,
		store: store,
		logger: logger.With(func(c zerolog.Context) zerolog.Context {
			return c.Str("component", "server")
		}),
	}
}

func (s Server) Run(ctx context.Context) error {
	addr := fmt.Sprintf(":%d", s.conf.Port)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen: %s: %w", addr, err)
	}

	s.logger.Info().
		Str("addr", addr).
		Int("max_conns", s.conf.MaxConns).
		Msg("server listening")

	go func() {
		<-ctx.Done()

		s.logger.Info().Msg("shutdown requested, closing listener")

		if err := ln.Close(); err != nil {
			s.logger.Warn().Err(err).Msg("close listener")
		}
	}()

	var (
		wg  sync.WaitGroup
		sem chan struct{}
	)

	if s.conf.MaxConns > 0 {
		sem = make(chan struct{}, s.conf.MaxConns)
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				s.logger.Info().Msg("waiting for active connections")

				return s.waitConns(&wg)
			}

			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				s.logger.Warn().Err(err).Msg("accept failed, retrying")
				time.Sleep(acceptRetryDelay)

				continue
			}

			return fmt.Errorf("accept: %w", err)
		}

		if !s.acquire(sem, conn) {
			continue
		}

		wg.Add(1)

		go func() {
			defer wg.Done()

			if sem != nil {
				defer func() {
					<-sem
				}()
			}

			s.handle(ctx, conn)
		}()
	}

}

func (s Server) acquire(sem chan struct{}, conn net.Conn) bool {
	if sem == nil {
		return true
	}

	select {
	case sem <- struct{}{}:
		return true
	default:
		s.logger.Warn().
			Str("remote", conn.RemoteAddr().String()).
			Int("max_conns", s.conf.MaxConns).
			Msg("connection rejected: max clients reached")

		if err := conn.SetWriteDeadline(time.Now().Add(s.conf.WriteTimeout)); err != nil {
			s.logger.Debug().Err(err).Msg("set write deadline for rejected connections")
		}
		if _, err := conn.Write([]byte("-ERR max number of clients reached\r\n")); err != nil {
			s.logger.Debug().Err(err).Msg("send max clients reply")
		}
		if err := conn.Close(); err != nil {
			s.logger.Debug().Err(err).Msg("close rejected connection")
		}

		return false
	}
}

func (s Server) waitConns(wg *sync.WaitGroup) error {
	done := make(chan struct{})

	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(s.conf.ShutdownTimeout):
		return ErrShutdown
	}
}

func (s Server) handle(ctx context.Context, conn net.Conn) {
	logg := s.logger.With(func(c zerolog.Context) zerolog.Context {
		return c.Str("remote", conn.RemoteAddr().String())
	})

	logg.Debug().Msg("connection opened")

	defer func() {
		logg.Debug().Msg("connection closed")
	}()

	defer conn.Close()

	done := make(chan struct{})
	defer close(done)

	go func() {
		select {
		case <-ctx.Done():
			if err := conn.Close(); err != nil {
				logg.Debug().Err(err).Msg("close connection on shutdown")
			}
		case <-done:
		}
	}()

	r := protocol.NewReader(conn)
	w := protocol.NewWriter(conn)

	/*
		idle window waits for a command, read window bounds in: one deadline
		would let a slow client hold the connections for IdleTimeout
	*/
	for {
		// wait for the next command (long idle window)
		if err := conn.SetReadDeadline(time.Now().Add(s.conf.IdleTimeout)); err != nil {
			logg.Warn().Err(err).Msg("set idle read deadline")
			return
		}

		if _, err := r.Peek(1); err != nil {
			switch {
			case errors.Is(err, os.ErrDeadlineExceeded):
				logg.Debug().
					Dur("idle_timeout", s.conf.IdleTimeout).
					Msg("idle timeout, closing connection")
			case errors.Is(err, io.EOF):
				logg.Debug().Msg("client closed connection")
			case errors.Is(err, net.ErrClosed):
				logg.Debug().Msg("connection closed on shutdown")
			default:
				logg.Debug().Err(err).Msg("read next command")
			}

			return
		}

		// command started arriving (short read window)
		if err := conn.SetReadDeadline(time.Now().Add(s.conf.ReadTimeout)); err != nil {
			logg.Warn().Err(err).Msg("set read deadline")
			return
		}

		cmd, err := protocol.ReadCommand(r)
		if err != nil {
			if errors.Is(err, protocol.ErrProtocol) {
				logg.Debug().Err(err).Msg("malformed command from client")

				if err := protocol.WriteError(w, "ERR Protocol error"); err != nil {
					logg.Debug().Err(err).Msg("send protocol error reply")
				}
				if err := w.Flush(); err != nil {
					logg.Debug().Err(err).Msg("flush protocol error reply")
				}
			}

			return
		}

		if cmd == nil {
			continue // empty inline line (ignored)
		}

		if err := conn.SetWriteDeadline(time.Now().Add(s.conf.WriteTimeout)); err != nil {
			logg.Warn().Err(err).Msg("set write deadline")
			return
		}

		cmdCtx, cancel := context.WithTimeout(ctx, s.conf.CmdTimeout)
		start := time.Now()
		err = s.exec(cmdCtx, w, cmd)
		cancel()

		took := time.Since(start)
		if s.conf.SlowCmdThreshold > 0 && took > s.conf.SlowCmdThreshold {
			logg.Warn().
				Str("cmd", string(cmd.Name)).
				Str("took", took.String()).
				Msg("slow command")
		}

		if err != nil {
			logg.Debug().Err(err).Str("cmd", string(cmd.Name)).Msg("write reply")

			return // connection dead
		}

		if err := w.Flush(); err != nil {
			logg.Debug().Err(err).Str("cmd", string(cmd.Name)).Msg("flush reply")
			return
		}
	}
}
