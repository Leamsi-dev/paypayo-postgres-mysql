#!/bin/bash
# monitor.sh

echo "=== PAYPAYO: APP DB LISTERNER MONITORING ==="
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