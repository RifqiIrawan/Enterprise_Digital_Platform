# Overlay: staging

**Thin scaffold, not deployable as-is.** Compare with `../dev`, which is
fully wired and actually runs today against a local cluster.

`kubectl kustomize infra/kubernetes/overlays/staging` renders without
errors, but the output still has every `REPLACE_ME_*` placeholder from
`../../base` unresolved, and no `jwt-secret` — applying it to a real cluster
would leave `auth-service`/`api-gateway` unable to start (fail-closed, by
design — see base's `README.md`) and every other service pointed at literal
hostnames like `REPLACE_ME_DATABASE_URL_FINANCE`.

## What this scaffold already has

- `edp-staging` namespace.
- Replica count bumped to 2 per service (not load-tested — just "more than
  one pod").

## What's still needed before this can actually be applied

1. **Real infra endpoints** — a `configMapGenerator` (`behavior: merge`,
   same pattern as `../dev/kustomization.yaml`) supplying real
   `DATABASE_URL` (staging Postgres, presumably `sslmode=require`),
   `KAFKA_BROKERS`, `REDIS_URL`, `CLICKHOUSE_URL`, `APP_ENV=staging`,
   `CORS_ALLOWED_ORIGIN`. See `infra/environments/staging/*.env.example`
   for the exact keys each service needs — those files were written as a
   reference for exactly this step.
2. **`jwt-secret`** — a `secretGenerator` (or, better, an External Secret /
   Sealed Secret pulling from a real secret manager rather than a literal
   in this file) providing `JWT_SECRET`. Must match whatever value
   `api-gateway` gets, since `auth-service` signs and `api-gateway`
   verifies with the same secret.
3. **A real image registry** — an `images:` transformer (same shape as
   `../dev/kustomization.yaml`) pointing `REPLACE_ME_REGISTRY/<service>` at
   wherever staging images actually get pushed. There's no CI/CD pipeline
   building/pushing these yet (no git remote configured for this repo) —
   that's separate, still-undone production-readiness work.
4. **Ingress host/class** — a patch (or full replacement) for
   `../../base/ingress.yaml`'s `REPLACE_ME_INGRESS_CLASS` and
   `REPLACE_ME_DOMAIN`, once there's an actual ingress controller and
   domain for staging.
5. Whether Postgres/Kafka/Redis/ClickHouse run in-cluster or stay
   external for staging hasn't been decided — base assumes external
   (matching this project's dev convention), but staging might reasonably
   run some of these in-cluster instead. Not addressed here.

None of the above can be filled in honestly without real staging
infrastructure existing first — this scaffold is deliberately left this
thin rather than inventing plausible-looking values for infrastructure that
doesn't exist.
