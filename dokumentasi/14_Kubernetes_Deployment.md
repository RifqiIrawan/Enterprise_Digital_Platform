# 14 — Kubernetes Deployment
## Enterprise Data Center Simulator (EDCS)

---

## ☸️ Overview

EDCS berjalan sepenuhnya di atas **Kubernetes (K8s)** — menggunakan **K3s** untuk lingkungan lokal/dev dan **full K8s** (EKS/GKE/AKS atau bare-metal via kubeadm) untuk staging dan production. Semua konfigurasi dikelola via **Helm Charts** dan di-deploy melalui **ArgoCD (GitOps)**.

---

## 🏗️ Cluster Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                     KUBERNETES CLUSTER                           │
│                                                                  │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │                 CONTROL PLANE                           │    │
│  │  API Server │ etcd │ Scheduler │ Controller Manager     │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                                  │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │  Node Pool:  │  │  Node Pool:  │  │  Node Pool:  │          │
│  │  System      │  │  Business    │  │  Data        │          │
│  │  (3 nodes)   │  │  (5 nodes)   │  │  (4 nodes)   │          │
│  │  4 CPU/8GB   │  │  8 CPU/16GB  │  │  16 CPU/64GB │          │
│  └──────────────┘  └──────────────┘  └──────────────┘          │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  Node Pool: GPU (2 nodes, optional — untuk ML Training)  │   │
│  │  8 CPU / 64 GB RAM / 1x NVIDIA A10G                      │   │
│  └──────────────────────────────────────────────────────────┘   │
└──────────────────────────────────────────────────────────────────┘
```

---

## 📦 Namespace Strategy

```yaml
Namespaces:
  edcs-system:        # Platform services (auth, gateway, monitoring)
  edcs-business:      # Business microservices
  edcs-data:          # Data platform (Kafka, Spark, Airflow)
  edcs-iot:           # IoT services (MQTT broker, device sim)
  edcs-ml:            # ML platform (MLflow, BentoML, JupyterHub)
  edcs-observability: # Monitoring stack
  edcs-devops:        # CI/CD tools (ArgoCD, Harbor)
  edcs-storage:       # Stateful sets (PostgreSQL, Redis, MinIO)
```

---

## 🔧 Helm Chart Structure (Per Service)

```
charts/
└── hris-service/
    ├── Chart.yaml
    ├── values.yaml              # Default values
    ├── values-staging.yaml      # Staging overrides
    ├── values-production.yaml   # Production overrides
    └── templates/
        ├── deployment.yaml
        ├── service.yaml
        ├── ingress.yaml
        ├── hpa.yaml             # Horizontal Pod Autoscaler
        ├── pdb.yaml             # Pod Disruption Budget
        ├── serviceaccount.yaml
        ├── configmap.yaml
        ├── secret.yaml          # References Vault
        └── NOTES.txt
```

### Chart.yaml
```yaml
apiVersion: v2
name: hris-service
description: EDCS HRIS Microservice
type: application
version: 1.0.0
appVersion: "1.2.3"
dependencies:
  - name: common
    repository: oci://ghcr.io/edcs/charts
    version: "1.x.x"
```

### values.yaml (Default)
```yaml
replicaCount: 2

image:
  repository: ghcr.io/edcs/hris-service
  pullPolicy: IfNotPresent
  tag: "latest"

service:
  type: ClusterIP
  port: 3003

ingress:
  enabled: true
  className: nginx
  annotations:
    nginx.ingress.kubernetes.io/rate-limit: "100"
    cert-manager.io/cluster-issuer: letsencrypt-prod
  hosts:
    - host: api.edcs.internal
      paths:
        - path: /hris
          pathType: Prefix
  tls:
    - secretName: edcs-tls
      hosts:
        - api.edcs.internal

resources:
  requests:
    memory: "256Mi"
    cpu: "250m"
  limits:
    memory: "512Mi"
    cpu: "500m"

autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 10
  targetCPUUtilizationPercentage: 70
  targetMemoryUtilizationPercentage: 80

podDisruptionBudget:
  enabled: true
  minAvailable: 1

livenessProbe:
  httpGet:
    path: /health
    port: 3003
  initialDelaySeconds: 30
  periodSeconds: 10
  failureThreshold: 3

readinessProbe:
  httpGet:
    path: /health/ready
    port: 3003
  initialDelaySeconds: 10
  periodSeconds: 5

env:
  NODE_ENV: production
  LOG_LEVEL: info
  KAFKA_BROKERS: kafka-0.kafka.edcs-data:9092,kafka-1.kafka.edcs-data:9092

envFromSecrets:
  - name: hris-db-secret
    key: DATABASE_URL
  - name: redis-secret
    key: REDIS_URL

tolerations: []
affinity:
  podAntiAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 100
        podAffinityTerm:
          labelSelector:
            matchExpressions:
              - key: app
                operator: In
                values:
                  - hris-service
          topologyKey: kubernetes.io/hostname

nodeSelector:
  pool: business
```

---

## 🗄️ StatefulSet: PostgreSQL per Service

```yaml
# charts/postgres-hris/templates/statefulset.yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: postgres-hris
  namespace: edcs-storage
spec:
  serviceName: postgres-hris
  replicas: 1  # Primary only (read replica opsional)
  selector:
    matchLabels:
      app: postgres-hris
  template:
    spec:
      containers:
      - name: postgres
        image: postgres:16
        env:
        - name: POSTGRES_DB
          value: hris_db
        - name: POSTGRES_USER
          valueFrom:
            secretKeyRef:
              name: postgres-hris-secret
              key: username
        - name: POSTGRES_PASSWORD
          valueFrom:
            secretKeyRef:
              name: postgres-hris-secret
              key: password
        - name: PGDATA
          value: /var/lib/postgresql/data/pgdata
        resources:
          requests:
            memory: "512Mi"
            cpu: "500m"
          limits:
            memory: "2Gi"
            cpu: "2"
        volumeMounts:
        - name: postgres-storage
          mountPath: /var/lib/postgresql/data
  volumeClaimTemplates:
  - metadata:
      name: postgres-storage
    spec:
      accessModes: ["ReadWriteOnce"]
      storageClassName: fast-ssd
      resources:
        requests:
          storage: 50Gi
```

---

## 📊 Kafka StatefulSet (KRaft Mode)

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: kafka
  namespace: edcs-data
spec:
  serviceName: kafka
  replicas: 3
  selector:
    matchLabels:
      app: kafka
  template:
    spec:
      containers:
      - name: kafka
        image: confluentinc/cp-kafka:7.5.0
        env:
        - name: KAFKA_NODE_ID
          valueFrom:
            fieldRef:
              fieldPath: metadata.annotations['kafka.node-id']
        - name: KAFKA_PROCESS_ROLES
          value: "broker,controller"
        - name: KAFKA_CONTROLLER_QUORUM_VOTERS
          value: "0@kafka-0.kafka:9093,1@kafka-1.kafka:9093,2@kafka-2.kafka:9093"
        - name: KAFKA_LISTENERS
          value: "PLAINTEXT://0.0.0.0:9092,CONTROLLER://0.0.0.0:9093"
        - name: KAFKA_LOG_DIRS
          value: /var/lib/kafka/data
        resources:
          requests:
            memory: "2Gi"
            cpu: "1"
          limits:
            memory: "4Gi"
            cpu: "2"
        volumeMounts:
        - name: kafka-storage
          mountPath: /var/lib/kafka/data
  volumeClaimTemplates:
  - metadata:
      name: kafka-storage
    spec:
      accessModes: ["ReadWriteOnce"]
      storageClassName: fast-ssd
      resources:
        requests:
          storage: 100Gi
```

---

## 🔐 Secrets Management (Vault + ESO)

```yaml
# External Secrets Operator — sync dari Vault ke K8s Secret
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: hris-db-secret
  namespace: edcs-business
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: vault-backend
    kind: ClusterSecretStore
  target:
    name: hris-db-secret
    creationPolicy: Owner
  data:
  - secretKey: DATABASE_URL
    remoteRef:
      key: edcs/hris
      property: database_url
  - secretKey: REDIS_URL
    remoteRef:
      key: edcs/shared
      property: redis_url
```

---

## 📈 Horizontal Pod Autoscaler + KEDA

```yaml
# KEDA — Scale berdasarkan Kafka lag
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: hris-consumer-scaler
  namespace: edcs-business
spec:
  scaleTargetRef:
    name: hris-service
  minReplicaCount: 2
  maxReplicaCount: 20
  triggers:
  - type: kafka
    metadata:
      bootstrapServers: kafka-0.kafka.edcs-data:9092
      consumerGroup: hris-consumer
      topic: hris.employee.created
      lagThreshold: "100"        # Scale up jika lag > 100 pesan
      offsetResetPolicy: latest
  - type: prometheus
    metadata:
      serverAddress: http://prometheus.edcs-observability:9090
      metricName: http_requests_per_second
      threshold: "100"
      query: sum(rate(http_requests_total{service="hris-service"}[1m]))
```

---

## 🌐 Ingress & Service Mesh

```yaml
# Kong Ingress Controller
apiVersion: configuration.konghq.com/v1
kind: KongPlugin
metadata:
  name: rate-limiting
config:
  minute: 100
  hour: 5000
  policy: local
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: edcs-api-gateway
  annotations:
    konghq.com/plugins: rate-limiting,jwt-auth,cors
    konghq.com/strip-path: "true"
spec:
  ingressClassName: kong
  rules:
  - host: api.edcs.internal
    http:
      paths:
      - path: /v1/hris
        pathType: Prefix
        backend:
          service:
            name: hris-service
            port:
              number: 3003
      - path: /v1/crm
        pathType: Prefix
        backend:
          service:
            name: crm-service
            port:
              number: 3005
```

---

## ⚡ Resource Quotas per Namespace

```yaml
apiVersion: v1
kind: ResourceQuota
metadata:
  name: edcs-business-quota
  namespace: edcs-business
spec:
  hard:
    requests.cpu: "20"
    requests.memory: "40Gi"
    limits.cpu: "40"
    limits.memory: "80Gi"
    pods: "100"
    services: "30"
    persistentvolumeclaims: "20"
```

---

## 🔄 Cluster Upgrade Strategy

| Step | Action | Rollback |
|------|--------|----------|
| 1 | Snapshot etcd | — |
| 2 | Drain node satu per satu | `kubectl uncordon` |
| 3 | Upgrade control plane | Restore etcd snapshot |
| 4 | Upgrade worker nodes (rolling) | `kubectl cordon` + drain |
| 5 | Verify semua workloads healthy | — |
| 6 | Run smoke tests | — |

**Frekuensi upgrade:** Mengikuti K8s N-2 support policy (max 2 minor version di belakang latest)
