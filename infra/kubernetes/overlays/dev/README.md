# Overlay: dev

The only overlay meant to actually work out of the box against a local
cluster today (Docker Desktop Kubernetes, or any cluster that can see
locally-built Docker images). `staging`/`prod` are thin scaffolds pending
real infrastructure — see their own `README.md`.

## Prerequisites

1. A local Kubernetes cluster with access to this machine's local Docker
   image store (Docker Desktop: Settings → Kubernetes → Enable Kubernetes).
2. Images built locally: `cd infra && docker compose build` (see root
   `infra/README.md`) — this overlay's `images:` transformer points at the
   `infra-<service>:latest` names that command produces.
3. Postgres running natively with all 13 databases created (same
   prerequisite as local dev without Docker — see root `infra/README.md`).
4. Kafka/Redis/ClickHouse running via `docker compose up -d clickhouse redis
   kafka` from `infra/` (MinIO and kafka-ui aren't needed for this).
   Pods reach these the same way a natively-run `go run ./cmd/server`
   process would: `host.docker.internal` at the same host-mapped ports.

## Deploy

```
kubectl apply -k infra/kubernetes/overlays/dev
kubectl -n edp-dev get pods -w
```

All 15 pods should reach `Running`/`1/1 Ready` within ~30s (liveness/readiness
probes hit `/health` on each service, `/` on frontend).

## Verify

No Ingress in this overlay (see kustomization.yaml) — use port-forward:

```
kubectl -n edp-dev port-forward svc/api-gateway 8079:8079
curl http://localhost:8079/health

kubectl -n edp-dev port-forward svc/frontend 3000:80
# open http://localhost:3000
```

Or check a cross-service call works end-to-end (same kind of check used to
validate the docker-compose stack):

```
kubectl -n edp-dev port-forward svc/ai-bi-service 8093:8093
curl "http://localhost:8093/dashboards/summary?company_id=<any-uuid>"
# "errors": [] confirms ai-bi-service reached all 8 other Services by name
```

## Tear down

```
kubectl delete -k infra/kubernetes/overlays/dev
```

(deletes the `edp-dev` namespace and everything in it).
