Analysons la capacité de l'application sur un serveur Debian avec **3 GB de RAM**. 📊

## 🎯 Analyse de la Charge Supportée

### Consommation Mémoire Estimée

| Composant | Mémoire | Description |
|-----------|---------|-------------|
| **Application Go** | ~50-100 MB | Binaire + runtime Go |
| **Pool de connexions DB** | ~10-30 MB | Connexions PostgreSQL/MySQL |
| **Workers (5)** | ~50 MB | Goroutines + buffers |
| **Channel d'événements** | ~5-10 MB | Buffer de 100 événements |
| **Logger** | ~5-10 MB | Buffer d'écriture |
| **OS Debian** | ~200-400 MB | Système de base |
| **Marge de sécurité** | ~500 MB | Pour pics de charge |
| **TOTAL** | **~1-1.5 GB** | Utilisation normale |

✅ **Reste disponible : ~1.5-2 GB** pour la base de données ou autres services

## 📈 Capacité de Traitement Estimée

### Scénario 1 : Configuration Actuelle (5 workers)

```yaml
worker:
  pool_size: 5
```

**Capacité :**
- **~50-100 événements/seconde** en traitement simultané
- **~3,000-6,000 événements/minute**
- **~180,000-360,000 événements/heure**
- **~4-8 millions d'événements/jour**

### Scénario 2 : Configuration Optimisée (10 workers)

```yaml
worker:
  pool_size: 10
```

**Capacité :**
- **~100-200 événements/seconde**
- **~6,000-12,000 événements/minute**
- **~360,000-720,000 événements/heure**
- **~8-17 millions d'événements/jour**

### Scénario 3 : Configuration Haute Performance (20 workers)

```yaml
worker:
  pool_size: 20
```

**Capacité :**
- **~200-400 événements/seconde**
- **~12,000-24,000 événements/minute**
- **~720,000-1,440,000 événements/heure**
- **~17-34 millions d'événements/jour**

## ⚙️ Facteurs Limitants

### 1. **Webhook externe** (Goulot d'étranglement principal)
- Si votre webhook répond en **100ms** → Max **10 requêtes/seconde/worker**
- Si votre webhook répond en **50ms** → Max **20 requêtes/seconde/worker**
- Si votre webhook répond en **20ms** → Max **50 requêtes/seconde/worker**

### 2. **Base de données**
- **PostgreSQL (LISTEN/NOTIFY)** : Quasi instantané, pas de limite
- **MySQL (Polling)** : Limité par `poll_interval` (toutes les 2 secondes par défaut)

### 3. **Réseau**
- Bande passante requise pour webhooks
- Latence vers le serveur webhook

## 🚀 Configuration Recommandée pour 3 GB RAM

### Configuration Conservatrice (Stable)

```yaml
database:
  type: "postgres"
  host: "localhost"
  port: 5432
  user: "${DB_USER}"
  password: "${DB_PASSWORD}"
  database: "prod_db"
  table: "users"
  sslmode: "require"

listener:
  modes: "insert,update,delete"
  poll_interval: 2

webhook:
  url: "${WEBHOOK_URL}"
  timeout: 30
  retry_count: 3
  retry_delay: 5

logging:
  file: "/var/log/app-db-listener/app.log"
  level: "info"

worker:
  pool_size: 10  # ✅ Bon équilibre
```

**Capacité : ~5-10 millions événements/jour**

### Configuration Haute Performance

```yaml
worker:
  pool_size: 20  # ✅ Plus de workers

webhook:
  timeout: 15     # ✅ Timeout plus court
  retry_count: 2  # ✅ Moins de retries

logging:
  level: "warn"   # ✅ Moins de logs = moins d'I/O
```

**Capacité : ~15-30 millions événements/jour**

## 📊 Benchmark Réaliste

Voici des estimations basées sur différents profils d'utilisation :

### Profil E-commerce (Charge Moyenne)
```
- 1,000 commandes/jour
- 3 événements par commande (insert order, update status, insert payment)
= 3,000 événements/jour
```
✅ **Votre serveur : Largement suffisant (utilisation <1%)**

### Profil SaaS (Charge Élevée)
```
- 50,000 utilisateurs actifs/jour
- 20 événements par utilisateur
= 1,000,000 événements/jour
```
✅ **Votre serveur : Très confortable (utilisation ~10-20%)**

### Profil IoT/Analytics (Charge Très Élevée)
```
- 1,000 devices
- 100 événements/device/heure
= 2,400,000 événements/jour
```
✅ **Votre serveur : Gérable avec configuration optimisée (utilisation ~50-70%)**

## 🔧 Optimisations pour Maximiser la Performance

### 1. Optimiser le Channel Buffer

```go
// Dans postgres.go et mysql.go
eventCh: make(chan *notifier.ChangeEvent, 1000)  // ⬆️ Augmenter de 100 à 1000
```

### 2. Optimiser la Pool de Connexions

```go
// Dans connection.go
db.SetMaxOpenConns(50)      // ⬆️ Augmenter de 25 à 50
db.SetMaxIdleConns(10)      // ⬆️ Augmenter de 5 à 10
```

### 3. Batch Processing pour MySQL

Pour MySQL avec polling, traiter plus d'événements par batch :

```go
// Dans mysql.go, fonction pollChanges
query := fmt.Sprintf(`
    SELECT id, operation, table_name, changed_at, data, old_data
    FROM %s
    WHERE processed = FALSE
    ORDER BY changed_at ASC
    LIMIT 500  -- ⬆️ Augmenter de 100 à 500
`, auditTable)
```

### 4. Logging Asynchrone

Ajouter un buffer pour les logs :

```go
// Dans logger.go
l := &Logger{
    file:   file,
    logger: log.New(io.Writer(bufio.NewWriterSize(file, 8192)), "", 0),
    level:  parseLevel(level),
}
```

## 🛡️ Monitoring et Limites

### Créer un Script de Monitoring

```bash
#!/bin/bash
# monitor.sh

echo "=== App DB Listener Monitoring ==="
echo ""

# Mémoire utilisée
echo "Mémoire utilisée par l'app:"
ps aux | grep app-db-listener | grep -v grep | awk '{print $6/1024 " MB"}'

# Nombre de connexions
echo ""
echo "Connexions actives:"
netstat -an | grep :5432 | wc -l  # PostgreSQL
netstat -an | grep :3306 | wc -l  # MySQL

# Taille du fichier log
echo ""
echo "Taille du log:"
du -h /var/log/app-db-listener/app.log

# Events traités (dernière heure)
echo ""
echo "Events traités (dernière heure):"
grep "Notification envoyée avec succès" /var/log/app-db-listener/app.log | grep "$(date '+%Y-%m-%d %H')" | wc -l

# Erreurs (dernière heure)
echo ""
echo "Erreurs (dernière heure):"
grep "ERROR" /var/log/app-db-listener/app.log | grep "$(date '+%Y-%m-%d %H')" | wc -l
```

### Alertes de Performance

Ajoutez des métriques dans votre application :

```go
// Ajouter un endpoint HTTP pour les métriques
func (pl *PostgresListener) startMetricsServer() {
    http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
        stats := map[string]interface{}{
            "queue_size": len(pl.eventCh),
            "queue_capacity": cap(pl.eventCh),
            "workers": pl.config.Worker.PoolSize,
        }
        json.NewEncoder(w).Encode(stats)
    })
    go http.ListenAndServe(":8080", nil)
}
```

## 🎯 Tableau Récapitulatif

| Configuration | Workers | Charge Max/Jour | Utilisation RAM | Use Case |
|---------------|---------|-----------------|-----------------|----------|
| **Légère** | 5 | 4-8M événements | ~800 MB | E-commerce, CMS |
| **Moyenne** ✅ | 10 | 8-17M événements | ~1 GB | SaaS, API |
| **Élevée** | 20 | 17-34M événements | ~1.2 GB | IoT, Analytics |
| **Maximum** | 30 | 25-50M événements | ~1.5 GB | Big Data |

## ⚠️ Signes de Saturation

Surveillez ces indicateurs :

```
❌ Channel plein : "Canal d'événements plein, événement perdu"
❌ Retries fréquents : "Tentative 3/3 pour l'événement"
❌ Timeouts webhook : "Erreur envoi webhook: timeout"
❌ RAM > 80% : Risque de swap
```

## 🎯 Conclusion

Avec **3 GB de RAM**, votre serveur peut confortablement gérer :

✅ **5-10 millions d'événements/jour** (configuration standard)  
✅ **15-30 millions d'événements/jour** (configuration optimisée)  
✅ **Jusqu'à 50 millions/jour** en pointe (avec optimisations poussées)

**Recommandation : Commencez avec 10 workers et ajustez selon vos logs !**