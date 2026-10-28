package config

import (
	"fmt"
	"time"

	"github.com/nikita-belan-01/key_value_storage/pkg/default_env"
)

type Server struct {
	Port             int
	MaxConns         int
	ReadTimeout      time.Duration
	WriteTimeout     time.Duration
	IdleTimeout      time.Duration
	ShutdownTimeout  time.Duration
	CmdTimeout       time.Duration
	DefaultTTL       time.Duration
	SlowCmdThreshold time.Duration
}

func newServer() (Server, error) {
	port, err := default_env.GetNumber("SERVER_PORT", 8080)
	if err != nil {
		return Server{}, err
	}

	maxConns, err := default_env.GetNumber("SERVER_MAX_CONNS", 0)
	if err != nil {
		return Server{}, err
	}

	if err = validatePort(port); err != nil {
		return Server{}, fmt.Errorf("SERVER_PORT: %w", err)
	}

	readTimeout, err := default_env.GetDuration("SERVER_READ_TIMEOUT", 15*time.Second)
	if err != nil {
		return Server{}, err
	}

	if err = validateDuration(readTimeout); err != nil {
		return Server{}, fmt.Errorf("SERVER_READ_TIMEOUT: %w", err)
	}

	idleTimeout, err := default_env.GetDuration("SERVER_IDLE_TIMEOUT", time.Minute)
	if err != nil {
		return Server{}, err
	}

	if err = validateDuration(idleTimeout); err != nil {
		return Server{}, fmt.Errorf("SERVER_IDLE_TIMEOUT: %w", err)
	}

	writeTimeout, err := default_env.GetDuration("SERVER_WRITE_TIMEOUT", 15*time.Second)
	if err != nil {
		return Server{}, err
	}

	if err = validateDuration(writeTimeout); err != nil {
		return Server{}, fmt.Errorf("SERVER_WRITE_TIMEOUT: %w", err)
	}

	shutdownTimeout, err := default_env.GetDuration("SERVER_SHUTDOWN_TIMEOUT", 5*time.Second)
	if err != nil {
		return Server{}, err
	}

	if err = validateDuration(shutdownTimeout); err != nil {
		return Server{}, fmt.Errorf("SERVER_SHUTDOWN_TIMEOUT: %w", err)
	}

	cmdTimeout, err := default_env.GetDuration("SERVER_CMD_TIMEOUT", 5*time.Second)
	if err != nil {
		return Server{}, err
	}

	if err = validateDuration(cmdTimeout); err != nil {
		return Server{}, fmt.Errorf("SERVER_CMD_TIMEOUT: %w", err)
	}

	slowCmdThreshold, err := default_env.GetDuration("SERVER_SLOW_CMD_THRESHOLD", 50*time.Millisecond)
	if err != nil {
		return Server{}, err
	}

	if err = validateDuration(shutdownTimeout); err != nil {
		return Server{}, fmt.Errorf("SERVER_SLOW_CMD_THRESHOLD: %w", err)
	}

	return Server{
		Port:             port,
		MaxConns:         maxConns,
		ReadTimeout:      readTimeout,
		WriteTimeout:     writeTimeout,
		IdleTimeout:      idleTimeout,
		ShutdownTimeout:  shutdownTimeout,
		CmdTimeout:       cmdTimeout,
		SlowCmdThreshold: slowCmdThreshold,
	}, nil
}
