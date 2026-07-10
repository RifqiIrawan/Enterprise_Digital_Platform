# 13 — CI/CD & DevOps
## Enterprise Data Center Simulator (EDCS)

---

## ⚙️ Overview

EDCS mengadopsi **GitOps** sebagai filosofi utama — semua konfigurasi infrastruktur dan deployment didefinisikan sebagai kode di Git. Pipeline CI/CD menggunakan **GitHub Actions** untuk build/test dan **ArgoCD** untuk deployment ke Kubernetes.

---

## 🔄 CI/CD Pipeline Flow

```
Developer Push → GitHub PR
        │
        ▼
┌─────────────────────────────────────────────────────────────┐
│                  CI PIPELINE (GitHub Actions)               │
│                                                             │
│  1. Lint & Format Check       (~1 menit)                   │
│  2. Unit Tests                (~3 menit)                   │
│  3. Integration Tests         (~5 menit)                   │
│  4. Security Scan (SAST)      (~3 menit)                   │
│  5. Build Docker Image        (~4 menit)                   │
│  6. Scan Image (Trivy)        (~2 menit)                   │
│  7. Push to Registry          (~1 menit)                   │
│  8. Update Helm values.yaml   (~1 menit)                   │
│                                 ─────────                  │
│                         Total: < 20 menit                  │
└──────────────────────────────────────┬──────────────────────┘
                                       │ (PR Merged ke main)
                                       ▼
┌─────────────────────────────────────────────────────────────┐
│                  CD PIPELINE (ArgoCD)                       │
│                                                             │
│  1. ArgoCD deteksi perubahan di Git                        │
│  2. Sync ke Staging namespace                               │
│  3. Smoke tests post-deploy                                 │
│  4. Manual approval (Production)                            │
│  5. Blue/Green deployment ke Production                     │
│  6. Health check & rollback otomatis jika gagal            │
└─────────────────────────────────────────────────────────────┘
```

---

## 📋 GitHub Actions Workflows

### Main CI Workflow
```yaml
# .github/workflows/ci.yml
name: CI Pipeline

on:
  pull_request:
    branches: [main, develop]
  push:
    branches: [main]

env:
  REGISTRY: ghcr.io
  IMAGE_NAME: ${{ github.repository }}

jobs:
  test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:15
        env:
          POSTGRES_DB: test_db
          POSTGRES_PASSWORD: test123
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
      redis:
        image: redis:7
        options: --health-cmd "redis-cli ping"
      kafka:
        image: confluentinc/cp-kafka:7.5.0
        env:
          KAFKA_ZOOKEEPER_CONNECT: zookeeper:2181
          KAFKA_ADVERTISED_LISTENERS: PLAINTEXT://localhost:9092

    steps:
      - uses: actions/checkout@v4

      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: '20'
          cache: 'npm'

      - name: Install dependencies
        run: npm ci

      - name: Lint
        run: npm run lint

      - name: Type check
        run: npm run type-check

      - name: Unit tests
        run: npm run test:unit -- --coverage

      - name: Integration tests
        run: npm run test:integration
        env:
          DATABASE_URL: postgresql://postgres:test123@localhost:5432/test_db
          REDIS_URL: redis://localhost:6379
          KAFKA_BROKERS: localhost:9092

      - name: Upload coverage
        uses: codecov/codecov-action@v3

  security:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: SAST (SonarQube)
        uses: SonarSource/sonarcloud-github-action@master
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          SONAR_TOKEN: ${{ secrets.SONAR_TOKEN }}

      - name: Dependency scan (npm audit)
        run: npm audit --audit-level=high

      - name: Secret scan (Gitleaks)
        uses: gitleaks/gitleaks-action@v2

  build:
    needs: [test, security]
    runs-on: ubuntu-latest
    outputs:
      image-tag: ${{ steps.meta.outputs.tags }}
      image-digest: ${{ steps.build.outputs.digest }}

    steps:
      - uses: actions/checkout@v4

      - name: Docker meta
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}
          tags: |
            type=sha,prefix=sha-
            type=ref,event=branch
            type=semver,pattern={{version}}

      - name: Build and push
        id: build
        uses: docker/build-push-action@v5
        with:
          context: .
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          cache-from: type=gha
          cache-to: type=gha,mode=max

      - name: Scan image (Trivy)
        uses: aquasecurity/trivy-action@master
        with:
          image-ref: ${{ steps.meta.outputs.tags }}
          severity: 'CRITICAL,HIGH'
          exit-code: '1'

  update-helm:
    needs: build
    runs-on: ubuntu-latest
    if: github.ref == 'refs/heads/main'
    steps:
      - uses: actions/checkout@v4
        with:
          repository: edcs/gitops-config
          token: ${{ secrets.GITOPS_TOKEN }}

      - name: Update image tag in Helm values
        run: |
          SERVICE_NAME=${{ github.event.repository.name }}
          NEW_TAG=${{ needs.build.outputs.image-tag }}
          yq e -i ".image.tag = \"${NEW_TAG}\"" \
            "apps/${SERVICE_NAME}/values-staging.yaml"

      - name: Commit and push
        run: |
          git config user.name "github-actions[bot]"
          git commit -am "ci: update ${SERVICE_NAME} to ${NEW_TAG}"
          git push
```

### Release to Production Workflow
```yaml
# .github/workflows/release.yml
name: Release to Production

on:
  workflow_dispatch:
    inputs:
      service:
        description: 'Service to deploy'
        required: true
      version:
        description: 'Version tag'
        required: true
      rollback_version:
        description: 'Rollback version (if needed)'
        required: false

jobs:
  approve:
    runs-on: ubuntu-latest
    environment: production   # Requires manual approval in GitHub
    steps:
      - name: Deployment approved
        run: echo "Deployment to production approved"

  deploy:
    needs: approve
    runs-on: ubuntu-latest
    steps:
      - name: Update production Helm values
        run: |
          yq e -i ".image.tag = \"${{ inputs.version }}\"" \
            "apps/${{ inputs.service }}/values-production.yaml"
          git commit -am "release: ${{ inputs.service }} v${{ inputs.version }}"
          git push

      - name: Wait for ArgoCD sync
        run: |
          argocd app wait ${{ inputs.service }} \
            --timeout 300 \
            --health

      - name: Run smoke tests
        run: npm run test:smoke -- --service=${{ inputs.service }}

      - name: Notify Slack
        uses: slackapi/slack-github-action@v1
        with:
          payload: |
            {"text": "✅ ${{ inputs.service }} v${{ inputs.version }} deployed to production"}
        env:
          SLACK_WEBHOOK_URL: ${{ secrets.SLACK_WEBHOOK }}
```

---

## 🚀 ArgoCD GitOps

### Application Definition
```yaml
# gitops-config/apps/hris-service/application.yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: hris-service
  namespace: argocd
  annotations:
    notifications.argoproj.io/subscribe.on-sync-succeeded.slack: deployments
    notifications.argoproj.io/subscribe.on-health-degraded.slack: alerts
spec:
  project: edcs-business
  source:
    repoURL: https://github.com/edcs/gitops-config
    targetRevision: HEAD
    path: apps/hris-service
    helm:
      valueFiles:
        - values-production.yaml
  destination:
    server: https://kubernetes.default.svc
    namespace: edcs-business
  syncPolicy:
    automated:
      prune: true           # Hapus resource yang dihapus dari Git
      selfHeal: true        # Auto-rollback jika drift dari Git
    syncOptions:
      - CreateNamespace=true
      - PrunePropagationPolicy=foreground
    retry:
      limit: 3
      backoff:
        duration: 30s
        factor: 2
        maxDuration: 3m
```

---

## 🔵🟢 Blue/Green Deployment Strategy

```yaml
# Helm chart dengan blue/green support
# values-production.yaml
deployment:
  strategy: blue-green

blueGreen:
  activeService: hris-active
  previewService: hris-preview
  autoPromotionEnabled: false    # Manual promotion
  autoPromotionSeconds: 300
  scaleDownDelaySeconds: 30
  prePromotionAnalysis:
    templates:
    - templateName: success-rate
    args:
    - name: service-name
      value: hris-preview
  postPromotionAnalysis:
    templates:
    - templateName: error-rate

# Argo Rollouts Analysis Template
apiVersion: argoproj.io/v1alpha1
kind: AnalysisTemplate
metadata:
  name: success-rate
spec:
  metrics:
  - name: success-rate
    interval: 1m
    count: 5
    successCondition: result[0] >= 0.95
    provider:
      prometheus:
        address: http://prometheus:9090
        query: |
          sum(rate(http_requests_total{service="{{args.service-name}}",
          status!~"5.."}[1m])) /
          sum(rate(http_requests_total{service="{{args.service-name}}"}[1m]))
```

---

## 🛡️ DevSecOps Integration

### Security Gates per Stage
| Stage | Tool | Gate Condition |
|-------|------|----------------|
| Pre-commit | Gitleaks | No secrets in code |
| PR | SonarQube | Quality Gate PASSED |
| PR | npm audit | No HIGH/CRITICAL vulns |
| Build | Trivy (image) | No CRITICAL vulns |
| Staging | DAST (OWASP ZAP) | No HIGH findings |
| Pre-prod | Pen Test | Quarterly |
| Runtime | Falco | Runtime security alerts |

### Falco Runtime Rules
```yaml
# Custom Falco rules untuk EDCS
- rule: Unexpected shell in production
  desc: Shell spawned in production container
  condition: >
    spawned_process and container
    and container.image.repository = "edcs/*"
    and proc.name in (bash, sh, zsh)
    and not proc.pname in (entrypoint.sh)
  output: "Shell spawned in EDCS container (user=%user.name proc=%proc.name)"
  priority: WARNING
  tags: [container, shell]
```

---

## 📊 DevOps Metrics (DORA)

| Metric | Target | Cara Ukur |
|--------|--------|-----------|
| Deployment Frequency | > 5x/hari | GitHub Actions deployments |
| Lead Time for Changes | < 1 jam | PR open → production |
| Mean Time to Recovery | < 30 menit | Incident → resolved |
| Change Failure Rate | < 5% | Failed deployments / total |
