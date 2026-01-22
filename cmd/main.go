package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"app-db-listener/internal/config"
	"app-db-listener/internal/database"
	"app-db-listener/internal/logger"
	"app-db-listener/internal/notifier"
)

func main() {
	configFile := flag.String("config", "config.yaml", "Chemin du fichier de configuration")
	flag.Parse()

	// Charger la configuration
	cfg, err := config.Load(*configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erreur chargement configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialiser le logger
	log, err := logger.New(cfg.Logging.File, cfg.Logging.Level)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erreur initialisation logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Close()

	log.Info("=== Démarrage de l'application DB Listener ===")
	log.Info("Type de base de données: %s", cfg.Database.Type)
	log.Info("Table surveillée: %s", cfg.Database.Table)
	log.Info("Modes activés: %s", cfg.Listener.Modes)
	log.Info("URL webhook: %s", cfg.Webhook.URL)

	// Initialiser le notifier
	ntf := notifier.New(&cfg.Webhook, log)

	// Initialiser le listener
	listener, err := database.NewListener(cfg, log, ntf)
	if err != nil {
		log.Error("Erreur initialisation listener: %v", err)
		os.Exit(1)
	}
	defer listener.Close()

	// Contexte avec annulation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Gérer les signaux d'arrêt
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Démarrer l'écoute dans une goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- listener.Listen(ctx)
	}()

	log.Info("Application démarrée et en écoute...")

	// Attendre un signal d'arrêt ou une erreur
	select {
	case <-sigCh:
		log.Info("Signal d'arrêt reçu, fermeture de l'application...")
		cancel()
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			log.Error("Erreur du listener: %v", err)
		}
	}

	log.Info("=== Application arrêtée ===")
}
