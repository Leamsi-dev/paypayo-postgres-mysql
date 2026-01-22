package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"app-db-listener/internal/config"
	"app-db-listener/internal/logger"
)

type ChangeEvent struct {
	Operation string                 `json:"operation"` // insert, update, delete
	Table     string                 `json:"table"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
	OldData   map[string]interface{} `json:"old_data,omitempty"` // Pour les updates
}

type Notifier struct {
	config *config.WebhookConfig
	logger *logger.Logger
	client *http.Client
}

func New(cfg *config.WebhookConfig, log *logger.Logger) *Notifier {
	return &Notifier{
		config: cfg,
		logger: log,
		client: &http.Client{
			Timeout: time.Duration(cfg.Timeout) * time.Second,
		},
	}
}

func (n *Notifier) Notify(event *ChangeEvent) error {
	jsonData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("erreur marshalling JSON: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= n.config.RetryCount; attempt++ {
		if attempt > 0 {
			n.logger.Info("Tentative %d/%d pour l'événement %s", attempt, n.config.RetryCount, event.Operation)
			time.Sleep(time.Duration(n.config.RetryDelay) * time.Second)
		}

		req, err := http.NewRequest("POST", n.config.URL, bytes.NewBuffer(jsonData))
		if err != nil {
			lastErr = fmt.Errorf("erreur création requête: %w", err)
			n.logger.Error("Erreur création requête: %v", err)
			continue
		}

		req.Header.Set("Content-Type", "application/json")

		resp, err := n.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("erreur envoi requête: %w", err)
			n.logger.Error("Erreur envoi webhook (tentative %d): %v", attempt+1, err)
			continue
		}

		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			n.logger.Info("Notification envoyée avec succès: %s sur table %s", event.Operation, event.Table)
			return nil
		}

		lastErr = fmt.Errorf("statut HTTP %d", resp.StatusCode)
		n.logger.Warn("Webhook retourné statut %d (tentative %d)", resp.StatusCode, attempt+1)
	}

	n.logger.Error("Échec notification après %d tentatives: %v", n.config.RetryCount+1, lastErr)
	return lastErr
}
