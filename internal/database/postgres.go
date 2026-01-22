package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lib/pq"

	"app-db-listener/internal/config"
	"app-db-listener/internal/logger"
	"app-db-listener/internal/notifier"
)

type PostgresListener struct {
	db       *sql.DB
	listener *pq.Listener
	config   *config.Config
	logger   *logger.Logger
	notifier *notifier.Notifier
	eventCh  chan *notifier.ChangeEvent
}

func NewPostgresListener(cfg *config.Config, log *logger.Logger, ntf *notifier.Notifier) (*PostgresListener, error) {
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Database,
		cfg.Database.SSLMode,
	)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("erreur connexion PostgreSQL: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("erreur ping PostgreSQL: %w", err)
	}

	listener := pq.NewListener(connStr, 10*time.Second, time.Minute, func(ev pq.ListenerEventType, err error) {
		if err != nil {
			log.Error("Événement listener: %v", err)
		}
	})

	pl := &PostgresListener{
		db:       db,
		listener: listener,
		config:   cfg,
		logger:   log,
		notifier: ntf,
		eventCh:  make(chan *notifier.ChangeEvent, 100),
	}

	if err := pl.setupTriggers(); err != nil {
		return nil, fmt.Errorf("erreur setup triggers: %w", err)
	}

	return pl, nil
}

func (pl *PostgresListener) setupTriggers() error {
	channelName := fmt.Sprintf("%s_changes", pl.config.Database.Table)

	// Créer la fonction trigger
	functionSQL := fmt.Sprintf(`
		CREATE OR REPLACE FUNCTION notify_%s_changes()
		RETURNS TRIGGER AS $$
		DECLARE
			payload JSON;
		BEGIN
			IF (TG_OP = 'DELETE') THEN
				payload = json_build_object(
					'operation', TG_OP,
					'table', TG_TABLE_NAME,
					'timestamp', NOW(),
					'data', row_to_json(OLD)
				);
			ELSIF (TG_OP = 'UPDATE') THEN
				payload = json_build_object(
					'operation', TG_OP,
					'table', TG_TABLE_NAME,
					'timestamp', NOW(),
					'data', row_to_json(NEW),
					'old_data', row_to_json(OLD)
				);
			ELSIF (TG_OP = 'INSERT') THEN
				payload = json_build_object(
					'operation', TG_OP,
					'table', TG_TABLE_NAME,
					'timestamp', NOW(),
					'data', row_to_json(NEW)
				);
			END IF;
			
			PERFORM pg_notify('%s', payload::text);
			
			RETURN NULL;
		END;
		$$ LANGUAGE plpgsql;
	`, pl.config.Database.Table, channelName)

	if _, err := pl.db.Exec(functionSQL); err != nil {
		return fmt.Errorf("erreur création fonction: %w", err)
	}

	// Supprimer les triggers existants
	dropTriggerSQL := fmt.Sprintf(`
		DROP TRIGGER IF EXISTS %s_insert_trigger ON %s;
		DROP TRIGGER IF EXISTS %s_update_trigger ON %s;
		DROP TRIGGER IF EXISTS %s_delete_trigger ON %s;
	`,
		pl.config.Database.Table, pl.config.Database.Table,
		pl.config.Database.Table, pl.config.Database.Table,
		pl.config.Database.Table, pl.config.Database.Table,
	)

	if _, err := pl.db.Exec(dropTriggerSQL); err != nil {
		return fmt.Errorf("erreur suppression triggers: %w", err)
	}

	// Créer les triggers selon la configuration
	if pl.config.Listener.IsInsertEnabled() {
		triggerSQL := fmt.Sprintf(`
			CREATE TRIGGER %s_insert_trigger
			AFTER INSERT ON %s
			FOR EACH ROW EXECUTE FUNCTION notify_%s_changes();
		`, pl.config.Database.Table, pl.config.Database.Table, pl.config.Database.Table)

		if _, err := pl.db.Exec(triggerSQL); err != nil {
			return fmt.Errorf("erreur création trigger INSERT: %w", err)
		}
		pl.logger.Info("Trigger INSERT créé pour la table %s", pl.config.Database.Table)
	}

	if pl.config.Listener.IsUpdateEnabled() {
		triggerSQL := fmt.Sprintf(`
			CREATE TRIGGER %s_update_trigger
			AFTER UPDATE ON %s
			FOR EACH ROW EXECUTE FUNCTION notify_%s_changes();
		`, pl.config.Database.Table, pl.config.Database.Table, pl.config.Database.Table)

		if _, err := pl.db.Exec(triggerSQL); err != nil {
			return fmt.Errorf("erreur création trigger UPDATE: %w", err)
		}
		pl.logger.Info("Trigger UPDATE créé pour la table %s", pl.config.Database.Table)
	}

	if pl.config.Listener.IsDeleteEnabled() {
		triggerSQL := fmt.Sprintf(`
			CREATE TRIGGER %s_delete_trigger
			AFTER DELETE ON %s
			FOR EACH ROW EXECUTE FUNCTION notify_%s_changes();
		`, pl.config.Database.Table, pl.config.Database.Table, pl.config.Database.Table)

		if _, err := pl.db.Exec(triggerSQL); err != nil {
			return fmt.Errorf("erreur création trigger DELETE: %w", err)
		}
		pl.logger.Info("Trigger DELETE créé pour la table %s", pl.config.Database.Table)
	}

	return nil
}

func (pl *PostgresListener) Listen(ctx context.Context) error {
	channelName := fmt.Sprintf("%s_changes", pl.config.Database.Table)

	if err := pl.listener.Listen(channelName); err != nil {
		return fmt.Errorf("erreur LISTEN: %w", err)
	}

	pl.logger.Info("Écoute démarrée sur le canal: %s", channelName)

	// Démarrer les workers
	for i := 0; i < pl.config.Worker.PoolSize; i++ {
		go pl.worker(ctx, i)
	}

	for {
		select {
		case <-ctx.Done():
			pl.logger.Info("Arrêt de l'écoute")
			return ctx.Err()
		case n := <-pl.listener.Notify:
			if n == nil {
				continue
			}

			var event notifier.ChangeEvent
			if err := json.Unmarshal([]byte(n.Extra), &event); err != nil {
				pl.logger.Error("Erreur unmarshalling notification: %v", err)
				continue
			}

			select {
			case pl.eventCh <- &event:
			default:
				pl.logger.Warn("Canal d'événements plein, événement perdu")
			}
		case <-time.After(90 * time.Second):
			go func() {
				pl.listener.Ping()
			}()
		}
	}
}

func (pl *PostgresListener) worker(ctx context.Context, id int) {
	pl.logger.Debug("Worker %d démarré", id)

	for {
		select {
		case <-ctx.Done():
			pl.logger.Debug("Worker %d arrêté", id)
			return
		case event := <-pl.eventCh:
			if err := pl.notifier.Notify(event); err != nil {
				pl.logger.Error("Worker %d: Erreur notification: %v", id, err)
			}
		}
	}
}

func (pl *PostgresListener) Close() error {
	if pl.listener != nil {
		pl.listener.Close()
	}
	if pl.db != nil {
		return pl.db.Close()
	}
	return nil
}
