# Paypayo: DB Listener - Application d'Écoute de Base de Données.

Paypayo: application pour surveiller les changements dans une table de base de données (PostgreSQL ou MySQL) et envoyer des notifications via webhook.

## Fonctionnalités

- ✅ Support PostgreSQL et MySQL
- ✅ Écoute des opérations: INSERT, UPDATE, DELETE (configurable)
- ✅ Notifications webhook avec retry automatique
- ✅ Traitement asynchrone non-bloquant avec pool de workers
- ✅ Logging complet des erreurs et événements
- ✅ Configuration flexible via YAML
- ✅ Gestion gracieuse des signaux d'arrêt

## Différences PostgreSQL vs MySQL

### PostgreSQL
- Utilise **LISTEN/NOTIFY** pour la détection en temps réel
- Notifications instantanées sans polling
- Plus performant et réactif
- Recommandé si possible

### MySQL
- Utilise une **table d'audit** avec polling
- Intervalle configurable (par défaut 2 secondes)
- Nécessite plus de ressources
- Solution de secours fiable

## Installation

```bash
# Cloner le projet
git clone <votre-repo>
cd db-listener

# Télécharger les dépendances
go mod download

# Compiler
go build -o app-db-listener cmd/main.go
```

## Configuration

Créez un fichier `config.yaml` :

```yaml
database:
  type: "postgres"  # ou "mysql"
  host: "localhost"
  port: 5432
  user: "votre_user"
  password: "votre_password"
  database: "votre_database"
  table: "votre_table"
  sslmode: "disable" # ou "prefer, require" en prod

listener:
  # Modes: insert, update, delete (séparés par des virgules)
  modes: "insert,update,delete"  # ou "insert" ou "update,delete" etc.
  poll_interval: 2  # Seulement pour MySQL

webhook:
  url: "https://webhook.site/votre-uuid"
  timeout: 10
  retry_count: 3
  retry_delay: 5

logging:
  file: "app.log"
  level: "info"  # debug, info, warn, error

worker:
  pool_size: 5
```

## Utilisation

```bash
# Démarrer avec la config par défaut
./app-db-listener

# Démarrer avec un fichier de config spécifique
./app-db-listener -config=/chemin/vers/config.yaml
```

## Format des Notifications Webhook

L'application envoie des requêtes POST au webhook configuré avec le format JSON suivant :

### INSERT
```json
{
  "operation": "INSERT",
  "table": "users",
  "timestamp": "2024-01-20T10:30:00Z",
  "data": {
    "id": 123,
    "name": "John Doe",
    "email": "john@example.com"
  }
}
```

### UPDATE
```json
{
  "operation": "UPDATE",
  "table": "users",
  "timestamp": "2024-01-20T10:35:00Z",
  "data": {
    "id": 123,
    "name": "John Smith",
    "email": "john@example.com"
  },
  "old_data": {
    "id": 123,
    "name": "John Doe",
    "email": "john@example.com"
  }
}
```

### DELETE
```json
{
  "operation": "DELETE",
  "table": "users",
  "timestamp": "2024-01-20T10:40:00Z",
  "data": {
    "id": 123,
    "name": "John Smith",
    "email": "john@example.com"
  }
}
```

## Modes d'Écoute

Vous pouvez configurer l'application pour écouter seulement certains types d'opérations :

```yaml
# Écouter uniquement les insertions
modes: "insert"

# Écouter uniquement les modifications
modes: "update"

# Écouter uniquement les suppressions
modes: "delete"

# Écouter insertions et modifications
modes: "insert,update"

# Écouter tous les événements
modes: "insert,update,delete"
```

## Logs

Les logs sont écrits dans le fichier spécifié dans la configuration (par défaut `app.log`).

Exemple de logs :
```
[2024-01-20 10:30:15] INFO: === Démarrage de l'application DB Listener ===
[2024-01-20 10:30:15] INFO: Type de base de données: postgres
[2024-01-20 10:30:15] INFO: Table surveillée: users
[2024-01-20 10:30:15] INFO: Modes activés: insert,update,delete
[2024-01-20 10:30:15] INFO: Trigger INSERT créé pour la table users
[2024-01-20 10:30:15] INFO: Écoute démarrée sur le canal: users_changes
[2024-01-20 10:30:20] INFO: Notification envoyée avec succès: INSERT sur table users
[2024-01-20 10:30:25] ERROR: Erreur envoi webhook (tentative 1): connection refused
```

## Test de l'Application

### 1. Tester avec webhook.site

```yaml
webhook:
  url: "https://webhook.site/votre-unique-id"
```

Visitez https://webhook.site pour obtenir une URL de test et voir les webhooks en temps réel.

### 2. Tester avec PostgreSQL

```sql
-- Créer une table de test
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100),
    email VARCHAR(100),
    created_at TIMESTAMP DEFAULT NOW()
);

-- Insérer des données
INSERT INTO users (name, email) VALUES ('John Doe', 'john@example.com');

-- Modifier des données
UPDATE users SET name = 'John Smith' WHERE id = 1;

-- Supprimer des données
DELETE FROM users WHERE id = 1;
```

### 3. Tester avec MySQL

```sql
-- Créer une table de test
CREATE TABLE users (
    id INT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(100),
    email VARCHAR(100),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Insérer des données
INSERT INTO users (name, email) VALUES ('John Doe', 'john@example.com');

-- Modifier des données
UPDATE users SET name = 'John Smith' WHERE id = 1;

-- Supprimer des données
DELETE FROM users WHERE id = 1;
```

## Nettoyage

### PostgreSQL
```sql
-- Supprimer les triggers
DROP TRIGGER IF EXISTS users_insert_trigger ON users;
DROP TRIGGER IF EXISTS users_update_trigger ON users;
DROP TRIGGER IF EXISTS users_delete_trigger ON users;

-- Supprimer la fonction
DROP FUNCTION IF EXISTS notify_users_changes();
```

### MySQL
```sql
-- Supprimer les triggers
DROP TRIGGER IF EXISTS users_insert_trigger;
DROP TRIGGER IF EXISTS users_update_trigger;
DROP TRIGGER IF EXISTS users_delete_trigger;

-- Supprimer la table d'audit
DROP TABLE IF EXISTS users_audit;
```

## Dépannage

### L'application ne démarre pas
- Vérifiez les paramètres de connexion dans `config.yaml`
- Vérifiez que la base de données est accessible
- Consultez les logs pour plus de détails

### Les notifications ne sont pas envoyées
- Vérifiez l'URL du webhook
- Vérifiez les logs pour voir les erreurs de retry
- Testez l'URL webhook manuellement avec curl

### Performance

Pour optimiser les performances :
- Ajustez `worker.pool_size` selon votre charge
- Pour MySQL, ajustez `poll_interval` (plus court = plus réactif mais plus de charge)
- Surveillez les logs pour détecter les canaux d'événements pleins

## Sécurité

- Ne commitez JAMAIS `config.yaml` avec des mots de passe réels
- Utilisez des variables d'environnement pour les secrets en production
- Utilisez SSL pour les connexions aux bases de données en production
- Protégez vos endpoints webhook avec authentification

## Licence

MIT