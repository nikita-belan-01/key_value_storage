package config

import "fmt"

type Config struct {
	Server Server
	Logger Logger
}

func New(confPath string) (*Config, error) {
	if err := loadEnv(confPath); err != nil {
		return nil, fmt.Errorf("load env by path: %s, %w", confPath, err)
	}

	server, err := newServer()
	if err != nil {
		return nil, err
	}

	logger, err := newLogger()
	if err != nil {
		return nil, err
	}

	return &Config{
		Server: server,
		Logger: logger,
	}, nil
}
