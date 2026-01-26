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

	// Afficher dans le terminal ET dans les logs
	banner := `
╔════════════════════════════════════════════════════════════════╗
║ 🚀 Paypayo: DB Listerner - application de surveillance de table ║
╚════════════════════════════════════════════════════════════════╝
`
	fmt.Println(banner)

	log.Info("=== Démarrage de l'application DB Listener ===")

	// Informations de configuration
	fmt.Printf("📊 Configuration:\n")
	fmt.Printf("   └─ Base de données : %s\n", cfg.Database.Type)
	fmt.Printf("   └─ Hôte           : %s:%d\n", cfg.Database.Host, cfg.Database.Port)
	fmt.Printf("   └─ Database       : %s\n", cfg.Database.Database)
	fmt.Printf("   └─ Table          : %s\n", cfg.Database.Table)
	fmt.Printf("   └─ SSL Mode       : %s\n", cfg.Database.SSLMode)
	fmt.Println()

	fmt.Printf("🎯 Modes d'écoute:\n")
	if cfg.Listener.IsInsertEnabled() {
		fmt.Printf("   ✅ INSERT activé\n")
	}
	if cfg.Listener.IsUpdateEnabled() {
		fmt.Printf("   ✅ UPDATE activé\n")
	}
	if cfg.Listener.IsDeleteEnabled() {
		fmt.Printf("   ✅ DELETE activé\n")
	}
	if cfg.Database.Type == "mysql" {
		fmt.Printf("   └─ Polling : toutes les %d secondes\n", cfg.Listener.PollInterval)
	}
	fmt.Println()

	fmt.Printf("🌐 Webhook:\n")
	fmt.Printf("   └─ URL     : %s\n", cfg.Webhook.URL)
	fmt.Printf("   └─ Timeout : %ds\n", cfg.Webhook.Timeout)
	fmt.Printf("   └─ Retries : %d tentatives\n", cfg.Webhook.RetryCount)
	fmt.Println()

	fmt.Printf("⚙️  Workers:\n")
	fmt.Printf("   └─ Pool size : %d workers\n", cfg.Worker.PoolSize)
	fmt.Println()

	fmt.Printf("📝 Logs:\n")
	fmt.Printf("   └─ Fichier : %s\n", cfg.Logging.File)
	fmt.Printf("   └─ Niveau  : %s\n", cfg.Logging.Level)
	fmt.Println()

	// Logger les mêmes infos
	log.Info("Type de base de données: %s", cfg.Database.Type)
	log.Info("Table surveillée: %s", cfg.Database.Table)
	log.Info("Modes activés: %s", cfg.Listener.Modes)
	log.Info("URL webhook: %s", cfg.Webhook.URL)
	log.Info("Workers: %d", cfg.Worker.PoolSize)

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
	fmt.Println("✨ Application démarrée avec succès!")
	fmt.Printf("👀 Surveillance active sur la table '%s'\n", cfg.Database.Table)
	fmt.Println("📡 En attente d'événements...")
	fmt.Println()
	fmt.Println("💡 Conseil: Pour exécuter en arrière-plan, utilisez 'nohup' ou 'systemd'")
	fmt.Println("   Exemple: nohup ./app-db-listener &")
	fmt.Println()
	fmt.Println("⏹️  Pour arrêter: Ctrl+C ou kill -SIGTERM <PID>")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Attendre un signal d'arrêt ou une erreur
	select {
	case <-sigCh:
		fmt.Println("\n🛑 Signal d'arrêt reçu...")
		log.Info("Signal d'arrêt reçu, fermeture de l'application...")
		cancel()
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			fmt.Printf("\n❌ Erreur: %v\n", err)
			log.Error("Erreur du listener: %v", err)
		}
	}

	fmt.Println("🔄 Fermeture en cours...")
	log.Info("=== Application arrêtée ===")
	fmt.Println("✅ Application arrêtée proprement")
	fmt.Println()
}
