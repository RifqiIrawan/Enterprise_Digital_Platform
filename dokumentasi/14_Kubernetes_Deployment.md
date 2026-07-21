# 14 — Kubernetes Deployment
## Enterprise Digital Platform (EDP)

---

## Overview

EDP menggunakan **Kustomize** (bukan Helm) untuk Kubernetes deployment. Manifest tersusun dalam base + 3 overlay (dev, staging, prod). Tidak ada ArgoCD, tidak ada KEDA, tidak ada Vault, tidak ada service mesh.

---

## Struktur Manifest

```
infra/kubernetes/
├── base/
│   ├── kustomization.yaml          # 14 service deployments + configmaps + ingress
│   ├── api-gateway.yaml            # Deployment + Service per service
│   ├── auth-service.yaml
│   ├── ... (14 file yaml total)
│   └── dw-service.yaml
│
└── overlays/
    ├── dev/
    │   ├── kustomization.yaml      # namespace=edp-dev, local images, host.docker.internal
    │   └── patch-ingress-delete.yaml  # Hapus Ingress (Docker Desktop K8s tidak punya ingress controller)
    ├── staging/
    │   └── kustomization.yaml      # namespace=edp-staging, replicas=2
    └── prod/
        └── kustomization.yaml      # namespace=edp-prod, replicas=3
```

---

## Base Layer

**52 resources** (setelah render): 15 Deployment, 15 Service, 15 ConfigMap, 1 Ingress, 6 resource tambahan (namespace, dll).

### Template Deployment (semua service sama)

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: finance-service
spec:
  replicas: 1
  selector:
    matchLabels:
      app: finance-service
  template:
    spec:
      containers:
      - name: finance-service
        image: REPLACE_ME_REGISTRY/finance-service:latest
        ports:
        - containerPort: 8085
        envFrom:
        - configMapRef:
            name: finance-service-config
        livenessProbe:
          httpGet:
            path: /health
            port: 8085
          initialDelaySeconds: 10
          periodSeconds: 30
        resources:
          requests:
            cpu: 50m
            memory: 64Mi
          limits:
            cpu: 250m
            memory: 256Mi
---
apiVersion: v1
kind: Service
metadata:
  name: finance-service
spec:
  selector:
    app: finance-service
  ports:
  - port: 8085
    targetPort: 8085
```

### ConfigMap per Service

Setiap service punya `{service}-config` ConfigMap dengan semua env var dari `config.go`:

```yaml
configMapGenerator:
- name: finance-service-config
  literals:
  - PORT=8085
  - DATABASE_URL=REPLACE_ME_DATABASE_URL_FINANCE
  - KAFKA_BROKERS=REPLACE_ME_KAFKA_BROKERS
  - FINANCE_SERVICE_URL=http://finance-service:8085  # lintas-service pakai K8s DNS
```

**Penting**: `JWT_SECRET` **tidak ada** di base ConfigMap (bahkan sebagai placeholder). Pod `auth-service` dan `api-gateway` akan gagal start kalau overlay tidak menyediakan Secret-nya (fail-closed by design).

### Ingress (di base, dihapus di overlay dev)

```yaml
spec:
  rules:
  - host: api.edp.local       # → api-gateway:8079
  - host: edp.local           # → frontend:80
```

---

## Overlay Dev

Untuk Docker Desktop Kubernetes (single-node, tidak ada ingress controller):

```yaml
namespace: edp-dev

# Image dari docker compose build (bukan registry)
images:
- name: REPLACE_ME_REGISTRY/finance-service
  newName: infra-finance-service
  newTag: latest

# Override ke host.docker.internal (Postgres + infra di luar Docker)
configMapGenerator:
- name: finance-service-config
  behavior: merge
  literals:
  - DATABASE_URL=postgres://platform:platform@host.docker.internal:5432/finance_service?sslmode=disable
  - KAFKA_BROKERS=host.docker.internal:9092

# Secret JWT (dev-only value)
secretGenerator:
- name: jwt-secret
  literals:
  - JWT_SECRET=dev-secret-change-me-in-prod

# Hapus Ingress (tidak ada ingress controller di Docker Desktop)
patches:
- target:
    kind: Ingress
  patch: |-
    - op: replace
      path: /metadata/name
      value: "$patch: delete"
```

---

## Overlay Staging & Prod

Minimal — cuma namespace dan replica count:

```yaml
# staging
namespace: edp-staging
patches:
- target: {kind: Deployment}
  patch: |-
    - op: replace
      path: /spec/replicas
      value: 2

# prod
namespace: edp-prod  
patches:
- target: {kind: Deployment}
  patch: |-
    - op: replace
      path: /spec/replicas
      value: 3
```

Yang belum ada di staging/prod: real ConfigMap values (butuh managed infra), Secrets dari secret manager, registry yang sesungguhnya, Ingress host yang valid.

---

## Deploy ke Docker Desktop

```bash
# Pastikan Kubernetes aktif di Docker Desktop
kubectl cluster-info

# Jalankan infra via docker-compose (Postgres native + Kafka/Redis/ClickHouse/MinIO via docker)
cd infra && docker compose up -d

# Apply K8s manifests
kubectl apply -k infra/kubernetes/overlays/dev

# Cek status
kubectl get pods -n edp-dev
# Semua harus 1/1 Running

# Akses via port-forward (tidak ada Ingress di dev)
kubectl port-forward -n edp-dev svc/api-gateway 8079:8079
kubectl port-forward -n edp-dev svc/frontend 3000:80

# Verifikasi
curl http://localhost:8079/health
curl http://localhost:3000

# Teardown
kubectl delete -k infra/kubernetes/overlays/dev
docker compose down
```

---

## dw-service di K8s — Catatan Khusus

`dw-service` koneksi ke ClickHouse dari dalam pod K8s memakai port host-remap (`9101`), bukan native port container (`9000`):

- **Dari go run (host)**: `CLICKHOUSE_ADDR=localhost:9101` (host port)
- **Dari docker-compose**: `CLICKHOUSE_ADDR=clickhouse:9000` (container port dalam network)
- **Dari pod K8s (dev overlay)**: `CLICKHOUSE_ADDR=host.docker.internal:9101` (host port via host gateway)

Ini karena ClickHouse berjalan di docker-compose network, dan pod K8s mengaksesnya lewat `host.docker.internal`, sama seperti proses native di host.
