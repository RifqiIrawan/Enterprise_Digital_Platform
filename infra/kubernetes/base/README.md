# Kubernetes Base Manifests

Plain Kustomize base (no Helm) — one `Deployment` + `Service` per service
(15 files: 14 Go services + frontend), one `Ingress` for external access, and
one `configMapGenerator` per Go service in `kustomization.yaml` for non-secret
config. Not meant to be applied directly — always go through an overlay
(`../overlays/{dev,staging,prod}/`), which supplies the namespace, the
environment-specific values for `REPLACE_ME_*` placeholders, and the
`jwt-secret` Secret that `auth-service`/`api-gateway` require.

## Design notes

- **Images**: every Deployment references `REPLACE_ME_REGISTRY/<service>:latest`.
  There's no image registry or CI/CD pipeline pushing images yet (no git
  remote configured for this repo), so this is a placeholder everywhere
  except the dev overlay, which patches it to the image names produced
  locally by `docker compose build` (see `infra/docker-compose.yml`) — no
  registry needed for a local cluster that shares the same Docker image
  store (Docker Desktop Kubernetes, or `kind --load`/`minikube image load`
  for other local clusters).
- **Cross-service URLs** use plain K8s Service DNS (`http://finance-service:8085`)
  — this resolves correctly in any namespace as long as that namespace has
  all the Services from this base deployed, which every overlay here does.
  These do NOT need per-environment overrides.
- **External infra** (`DATABASE_URL`, `KAFKA_BROKERS`, `REDIS_URL`,
  `CLICKHOUSE_URL`) are `REPLACE_ME_*` placeholders in base, since these
  point outside the cluster (Postgres is native, not containerized — see
  root `infra/README.md`) or to infra that may or may not run inside the
  same cluster depending on environment. Each overlay overrides these via a
  `configMapGenerator` with `behavior: merge`.
- **Secrets**: `jwt-secret` is deliberately absent from base entirely (not
  even as a placeholder) — `auth-service` and `api-gateway` both declare
  `envFrom.secretRef.name: jwt-secret` in their Deployment, so a pod fails
  to start with a clear error if no overlay supplies one, instead of
  silently running with something insecure.
- **Health probes**: every Go service already exposes `GET /health` (used
  throughout this project's own verification steps), so liveness/readiness
  both point there. `frontend` (static nginx) probes `/` instead.
- **Resources**: modest defaults (`50m`/`64Mi` request, `250m`/`256Mi`
  limit per Go service) sized for a lightweight CRUD service, not tuned
  against any real load test.
- **No StatefulSets for Postgres/Kafka/Redis/ClickHouse** — those stay
  external/native to the cluster in every environment defined here, same
  as `infra/docker-compose.yml`'s local-dev setup. Running them in-cluster
  (StatefulSets, PVCs, etc.) is a bigger, separate decision not made here.

## Rendering

```
kubectl kustomize infra/kubernetes/overlays/dev
```

(or `staging`/`prod`). This only renders YAML — no cluster needed. See each
overlay's own `README.md` for how to actually apply it.
