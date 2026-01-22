package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"app-db-listener/internal/config"
	"app-db-listener/internal/logger"
	"app-db-listener/internal/notifier"
)

type MySQLListener struct {
	db       *sql.DB
	config   *config.Config
	logger   *logger.Logger
	notifier *notifier.Notifier
	eventCh  chan *notifier.ChangeEvent
}

func NewMySQLListener(cfg *config.Config, log *logger.Logger, ntf *notifier.Notifier) (*MySQLListener, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.Database,
	)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("erreur connexion MySQL: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("erreur ping MySQL: %w", err)
	}

	ml := &MySQLListener{
		db:       db,
		config:   cfg,
		logger:   log,
		notifier: ntf,
		eventCh:  make(chan *notifier.ChangeEvent, 100),
	}

	if err := ml.setupAuditTable(); err != nil {
		return nil, fmt.Errorf("erreur setup audit: %w", err)
	}

	return ml, nil
}

func (ml *MySQLListener) setupAuditTable() error {
	auditTable := fmt.Sprintf("%s_audit", ml.config.Database.Table)

	// Créer la table d'audit
	createTableSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			operation VARCHAR(10) NOT NULL,
			table_name VARCHAR(255) NOT NULL,
			changed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			data JSON,
			old_data JSON,
			processed BOOLEAN DEFAULT FALSE,
			INDEX idx_processed (processed, changed_at)
		)
	`, auditTable)

	if _, err := ml.db.Exec(createTableSQL); err != nil {
		return fmt.Errorf("erreur création table audit: %w", err)
	}

	ml.logger.Info("Table d'audit créée: %s", auditTable)

	// Supprimer les triggers existants
	dropTriggers := []string{
		fmt.Sprintf("DROP TRIGGER IF EXISTS %s_insert_trigger", ml.config.Database.Table),
		fmt.Sprintf("DROP TRIGGER IF EXISTS %s_update_trigger", ml.config.Database.Table),
		fmt.Sprintf("DROP TRIGGER IF EXISTS %s_delete_trigger", ml.config.Database.Table),
	}

	for _, dropSQL := range dropTriggers {
		if _, err := ml.db.Exec(dropSQL); err != nil {
			ml.logger.Warn("Erreur suppression trigger: %v", err)
		}
	}

	// Créer les triggers selon la configuration
	if ml.config.Listener.IsInsertEnabled() {
		triggerSQL := fmt.Sprintf(`
			CREATE TRIGGER %s_insert_trigger
			AFTER INSERT ON %s
			FOR EACH ROW
			INSERT INTO %s (operation, table_name, data)
			VALUES ('INSERT', '%s', JSON_OBJECT(%s))
		`, ml.config.Database.Table, ml.config.Database.Table, auditTable,
			ml.config.Database.Table, ml.buildColumnList("NEW"))

		if _, err := ml.db.Exec(triggerSQL); err != nil {
			return fmt.Errorf("erreur création trigger INSERT: %w", err)
		}
		ml.logger.Info("Trigger INSERT créé pour la table %s", ml.config.Database.Table)
	}

	if ml.config.Listener.IsUpdateEnabled() {
		triggerSQL := fmt.Sprintf(`
			CREATE TRIGGER %s_update_trigger
			AFTER UPDATE ON %s
			FOR EACH ROW
			INSERT INTO %s (operation, table_name, data, old_data)
			VALUES ('UPDATE', '%s', JSON_OBJECT(%s), JSON_OBJECT(%s))
		`, ml.config.Database.Table, ml.config.Database.Table, auditTable,
			ml.config.Database.Table, ml.buildColumnList("NEW"), ml.buildColumnList("OLD"))

		if _, err := ml.db.Exec(triggerSQL); err != nil {
			return fmt.Errorf("erreur création trigger UPDATE: %w", err)
		}
		ml.logger.Info("Trigger UPDATE créé pour la table %s", ml.config.Database.Table)
	}

	if ml.config.Listener.IsDeleteEnabled() {
		triggerSQL := fmt.Sprintf(`
			CREATE TRIGGER %s_delete_trigger
			AFTER DELETE ON %s
			FOR EACH ROW
			INSERT INTO %s (operation, table_name, data)
			VALUES ('DELETE', '%s', JSON_OBJECT(%s))
		`, ml.config.Database.Table, ml.config.Database.Table, auditTable,
			ml.config.Database.Table, ml.buildColumnList("OLD"))

		if _, err := ml.db.Exec(triggerSQL); err != nil {
			return fmt.Errorf("erreur création trigger DELETE: %w", err)
		}
		ml.logger.Info("Trigger DELETE créé pour la table %s", ml.config.Database.Table)
	}

	return nil
}

func (ml *MySQLListener) buildColumnList(prefix string) string {
	// Récupérer les colonnes de la table
	rows, err := ml.db.Query(fmt.Sprintf("DESCRIBE %s", ml.config.Database.Table))
	if err != nil {
		ml.logger.Error("Erreur récupération colonnes: %v", err)
		return ""
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var field, typ, null, key, def, extra sql.NullString
		if err := rows.Scan(&field, &typ, &null, &key, &def, &extra); err != nil {
			continue
		}
		columns = append(columns, fmt.Sprintf("'%s', %s.%s", field.String, prefix, field.String))
	}

	result := ""
	for i, col := range columns {
		if i > 0 {
			result += ", "
		}
		result += col
	}
	return result
}

func (ml *MySQLListener) Listen(ctx context.Context) error {
	ml.logger.Info("Écoute démarrée sur la table: %s (polling chaque %d secondes)",
		ml.config.Database.Table, ml.config.Listener.PollInterval)

	// Démarrer les workers
	for i := 0; i < ml.config.Worker.PoolSize; i++ {
		go ml.worker(ctx, i)
	}

	ticker := time.NewTicker(time.Duration(ml.config.Listener.PollInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			ml.logger.Info("Arrêt de l'écoute")
			return ctx.Err()
		case <-ticker.C:
			if err := ml.pollChanges(ctx); err != nil {
				ml.logger.Error("Erreur polling: %v", err)
			}
		}
	}
}

func (ml *MySQLListener) pollChanges(ctx context.Context) error {
	auditTable := fmt.Sprintf("%s_audit", ml.config.Database.Table)

	// Récupérer les changements non traités
	query := fmt.Sprintf(`
		SELECT id, operation, table_name, changed_at, data, old_data
		FROM %s
		WHERE processed = FALSE
		ORDER BY changed_at ASC
		LIMIT 100
	`, auditTable)

	rows, err := ml.db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("erreur query audit: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var (
			id        int64
			operation string
			tableName string
			changedAt time.Time
			data      []byte
			oldData   sql.NullString
		)

		if err := rows.Scan(&id, &operation, &tableName, &changedAt, &data, &oldData); err != nil {
			ml.logger.Error("Erreur scan row: %v", err)
			continue
		}

		var dataMap map[string]interface{}
		if err := json.Unmarshal(data, &dataMap); err != nil {
			ml.logger.Error("Erreur unmarshal data: %v", err)
			continue
		}

		event := &notifier.ChangeEvent{
			Operation: operation,
			Table:     tableName,
			Timestamp: changedAt,
			Data:      dataMap,
		}

		if oldData.Valid {
			var oldDataMap map[string]interface{}
			if err := json.Unmarshal([]byte(oldData.String), &oldDataMap); err == nil {
				event.OldData = oldDataMap
			}
		}

		select {
		case ml.eventCh <- event:
			ids = append(ids, id)
		default:
			ml.logger.Warn("Canal d'événements plein")
		}
	}

	// Marquer comme traité
	if len(ids) > 0 {
		if err := ml.markProcessed(ids); err != nil {
			ml.logger.Error("Erreur marquage processed: %v", err)
		}
	}

	return nil
}

func (ml *MySQLListener) markProcessed(ids []int64) error {
	auditTable := fmt.Sprintf("%s_audit", ml.config.Database.Table)

	query := fmt.Sprintf("UPDATE %s SET processed = TRUE WHERE id IN (?", auditTable)
	args := make([]interface{}, len(ids))
	args[0] = ids[0]

	for i := 1; i < len(ids); i++ {
		query += ",?"
		args[i] = ids[i]
	}
	query += ")"

	_, err := ml.db.Exec(query, args...)
	return err
}

func (ml *MySQLListener) worker(ctx context.Context, id int) {
	ml.logger.Debug("Worker %d démarré", id)

	for {
		select {
		case <-ctx.Done():
			ml.logger.Debug("Worker %d arrêté", id)
			return
		case event := <-ml.eventCh:
			if err := ml.notifier.Notify(event); err != nil {
				ml.logger.Error("Worker %d: Erreur notification: %v", id, err)
			}
		}
	}
}

func (ml *MySQLListener) Close() error {
	if ml.db != nil {
		return ml.db.Close()
	}
	return nil
}
