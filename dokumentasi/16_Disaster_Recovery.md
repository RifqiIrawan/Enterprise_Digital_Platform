# 16 — Disaster Recovery
## Enterprise Data Center Simulator (EDCS)

---

## 🛡️ Overview

EDCS mengimplementasikan strategi **DR multi-tier** dengan RTO < 1 jam dan RPO < 15 menit untuk komponen kritis. Backup dilakukan secara otomatis dan DR drill wajib dijalankan setiap kuartal.

---

## 🎯 Recovery Objectives per Tier

| Tier | Komponen | RTO | RPO | Strategi |
|------|----------|-----|-----|----------|
| **Tier 0 (Critical)** | Auth, API Gateway, Finance | < 15 menit | < 5 menit | Active-Active Multi-AZ |
| **Tier 1 (High)** | HRIS, CRM, Sales, WMS | < 1 jam | < 15 menit | Active-Passive + Streaming Replication |
| **Tier 2 (Medium)** | MES, Procurement, Asset | < 4 jam | < 1 jam | Backup + Restore |
| **Tier 3 (Low)** | Reporting, Analytics, BI | < 24 jam | < 4 jam | Backup + Restore |

---

## 🗄️ Database Backup Strategy

### PostgreSQL Continuous Backup (WAL-G)
```bash
#!/bin/bash
# scripts/backup/postgres_backup.sh

SERVICE=$1
DB_NAME="${SERVICE}_db"
S3_BUCKET="s3://edcs-backups/postgres/${SERVICE}"

# Full backup harian (02:00 WIB)
backup_full() {
  BACKUP_NAME="full_$(date +%Y%m%d_%H%M%S)"
  echo "[$(date)] Starting full backup: ${BACKUP_NAME}"

  WALG_S3_PREFIX="${S3_BUCKET}" \
  PGHOST="postgres-${SERVICE}" \
  PGUSER="${DB_USER}" \
  PGPASSWORD="${DB_PASS}" \
  wal-g backup-push /var/lib/postgresql/data

  echo "[$(date)] Full backup completed: ${BACKUP_NAME}"

  # Cleanup backup > 30 hari
  WALG_S3_PREFIX="${S3_BUCKET}" wal-g delete retain FULL 30 --confirm
}

# WAL streaming (setiap transaksi)
enable_wal_archiving() {
  psql -c "ALTER SYSTEM SET archive_mode = on;"
  psql -c "ALTER SYSTEM SET archive_command = 'wal-g wal-push %p';"
  psql -c "ALTER SYSTEM SET archive_timeout = '60';"  # Flush setiap 60 detik
  psql -c "SELECT pg_reload_conf();"
}

# Point-in-Time Recovery (PITR)
restore_to_point_in_time() {
  TARGET_TIME=$1  # Format: "2026-07-09 03:00:00"
  echo "[$(date)] Starting PITR to: ${TARGET_TIME}"

  WALG_S3_PREFIX="${S3_BUCKET}" \
  wal-g backup-fetch /var/lib/postgresql/data LATEST

  cat > /var/lib/postgresql/data/recovery.conf <<EOF
restore_command = 'wal-g wal-fetch %f %p'
recovery_target_time = '${TARGET_TIME}'
recovery_target_action = promote
EOF

  pg_ctl start
  echo "[$(date)] PITR completed"
}
```

### Backup Schedule
```yaml
# kubernetes/cronjobs/backup-schedule.yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: postgres-full-backup
  namespace: edcs-system
spec:
  schedule: "0 2 * * *"      # 02:00 WIB setiap hari
  concurrencyPolicy: Forbid
  successfulJobsHistoryLimit: 7
  failedJobsHistoryLimit: 3
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: backup
            image: edcs/backup-tool:latest
            command: ["/scripts/backup_all_dbs.sh"]
            env:
            - name: S3_BUCKET
              value: "s3://edcs-backups"
            - name: RETENTION_DAYS
              value: "30"
          restartPolicy: OnFailure
---
# WAL-G WAL backup (continuous — via PostgreSQL archive_command)
# Tidak perlu CronJob terpisah, berjalan setiap transaksi selesai
```

---

## ☁️ Multi-AZ / Multi-Region Setup

### Active-Passive Architecture (Tier 1)
```
┌─────────────────────────────────────────────────────────┐
│                PRIMARY REGION (ap-southeast-1)          │
│                                                         │
│  ┌──────────────────────────────────────────────────┐   │
│  │  AZ-1: edcs-prod-1a                              │   │
│  │  PostgreSQL Primary │ App Pods │ Kafka Broker 0   │   │
│  └──────────────────────────────────────────────────┘   │
│  ┌──────────────────────────────────────────────────┐   │
│  │  AZ-2: edcs-prod-1b                              │   │
│  │  PostgreSQL Replica │ App Pods │ Kafka Broker 1   │   │
│  └──────────────────────────────────────────────────┘   │
│  ┌──────────────────────────────────────────────────┐   │
│  │  AZ-3: edcs-prod-1c                              │   │
│  │  PostgreSQL Replica │ App Pods │ Kafka Broker 2   │   │
│  └──────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
         │ WAL Streaming Replication (async)
         │ Kafka MirrorMaker 2
         ▼
┌─────────────────────────────────────────────────────────┐
│              DR REGION (ap-southeast-3)                 │
│                                                         │
│  ┌──────────────────────────────────────────────────┐   │
│  │  Standby PostgreSQL (lag < 15 menit)              │   │
│  │  Kafka Mirror Cluster                             │   │
│  │  Pre-scaled App Pods (paused)                    │   │
│  └──────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
```

### Kafka MirrorMaker 2 Config
```yaml
# Replicate semua topics dari primary ke DR
apiVersion: kafka.strimzi.io/v1beta2
kind: KafkaMirrorMaker2
metadata:
  name: edcs-mirror
spec:
  version: 3.5.0
  replicas: 2
  connectCluster: "dr-cluster"
  clusters:
    - alias: "primary"
      bootstrapServers: kafka-primary.ap-southeast-1:9092
    - alias: "dr-cluster"
      bootstrapServers: kafka-dr.ap-southeast-3:9092
  mirrors:
    - sourceCluster: "primary"
      targetCluster: "dr-cluster"
      sourceConnector:
        config:
          replication.factor: 3
          sync.topic.acls.enabled: false
          replication.policy.separator: ""     # Tidak tambah prefix ke topic name
      topicsPattern: ".*"                      # Mirror semua topics
      groupsPattern: ".*"                      # Mirror semua consumer groups
```

---

## 🔄 Failover Runbook

### Automatic Failover (Tier 0)
```bash
#!/bin/bash
# Dieksekusi otomatis oleh Alertmanager webhook jika primary down

set -euo pipefail
COMPONENT=$1
REASON=$2

log() { echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] $*" | tee -a /var/log/dr/failover.log; }
notify_slack() { curl -s -X POST "$SLACK_WEBHOOK" -d "{\"text\":\"$*\"}"; }

log "=== FAILOVER INITIATED: ${COMPONENT} — ${REASON} ==="
notify_slack "🚨 FAILOVER STARTED: ${COMPONENT} in primary region down. Initiating failover..."

case $COMPONENT in
  "auth-service")
    # Auth aktif di semua region via active-active
    log "Auth service: Global load balancer akan route ke DR region otomatis"
    kubectl --context=dr-cluster scale deployment auth-service --replicas=3
    ;;

  "finance-service")
    # Promote PostgreSQL replica di DR
    log "Step 1: Promote PostgreSQL replica di DR region"
    kubectl --context=dr-cluster exec postgres-finance-0 -- \
      pg_ctl promote -D /var/lib/postgresql/data

    log "Step 2: Update connection string ke DR database"
    kubectl --context=dr-cluster create secret generic finance-db-secret \
      --from-literal=DATABASE_URL="postgresql://user:pass@postgres-finance-dr:5432/finance_db" \
      --dry-run=client -o yaml | kubectl apply -f -

    log "Step 3: Scale up finance service di DR"
    kubectl --context=dr-cluster scale deployment finance-service --replicas=3

    log "Step 4: Update DNS/load balancer ke DR"
    # Route53 / Cloudflare API call to update DNS
    ;;
esac

notify_slack "✅ FAILOVER COMPLETED: ${COMPONENT} now serving from DR region"
log "=== FAILOVER COMPLETED ==="
```

### Manual Failover Checklist (Tier 1)
```markdown
## Pre-Failover
- [ ] Konfirmasi primary region tidak recoverable dalam RTO
- [ ] Approval dari: CTO + Head of Ops
- [ ] Notifikasi ke semua tim via #incident channel
- [ ] Cek lag WAL replication (harus < RPO threshold)

## Failover Steps
- [ ] 1. Hentikan write ke primary (maintenance mode)
- [ ] 2. Tunggu DR replica catch-up (lag = 0)
- [ ] 3. Promote PostgreSQL replica di DR: `pg_ctl promote`
- [ ] 4. Update Vault secrets dengan DR connection strings
- [ ] 5. Scale up app pods di DR Kubernetes cluster
- [ ] 6. Update DNS (TTL rendah — 60s untuk crisis readiness)
- [ ] 7. Verify health checks semua services di DR
- [ ] 8. Run smoke tests end-to-end

## Post-Failover
- [ ] Monitor error rates di DR (target: sama dengan primary)
- [ ] Informasikan users bahwa layanan telah dipindah
- [ ] Dokumentasikan timeline insiden
- [ ] Mulai persiapan failback ke primary
```

---

## 🧪 DR Drill Schedule

| Drill | Frekuensi | Scope | Target |
|-------|-----------|-------|--------|
| Backup restore test | Bulanan | 1 service random | RTO verified |
| Failover drill (staging) | Bulanan | Full platform | RTO < SLA |
| Failover drill (production) | Kuartalan | 1 Tier 1 service | RTO < SLA |
| Full DR simulation | Tahunan | Semua service | Business continuity |
| Chaos engineering | Mingguan | Random component | Resilience check |

---

## 📊 Backup Inventory

| Data | Metode | Frekuensi | Retention | Location |
|------|--------|-----------|-----------|----------|
| PostgreSQL (semua) | WAL-G continuous | Realtime WAL | 30 hari | S3 primary + DR |
| Redis | RDB snapshot | 1 jam | 7 hari | S3 |
| Kafka | MirrorMaker 2 | Realtime | 7 hari (DR) | DR region |
| MinIO (Data Lake) | S3 replication | Async | 90 hari | DR region |
| Elasticsearch | Snapshot API | Harian | 14 hari | S3 |
| Kubernetes configs | etcd backup | Harian | 30 hari | S3 |
| Secrets (Vault) | Vault snapshot | Harian | 30 hari | S3 encrypted |
