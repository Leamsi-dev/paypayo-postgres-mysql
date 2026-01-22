package database

import (
	"context"
	"fmt"

	"app-db-listener/internal/config"
	"app-db-listener/internal/logger"
	"app-db-listener/internal/notifier"
)

type Listener interface {
	Listen(ctx context.Context) error
	Close() error
}

func NewListener(cfg *config.Config, log *logger.Logger, ntf *notifier.Notifier) (Listener, error) {
	switch cfg.Database.Type {
	case "postgres":
		return NewPostgresListener(cfg, log, ntf)
	case "mysql":
		return NewMySQLListener(cfg, log, ntf)
	default:
		return nil, fmt.Errorf("type de base de données non supporté: %s", cfg.Database.Type)
	}
}
