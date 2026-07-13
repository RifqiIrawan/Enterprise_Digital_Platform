# Overlay: prod

**Thin scaffold, not deployable as-is.** Compare with `../dev`, which is
fully wired and actually runs today against a local cluster.

`kubectl kustomize infra/kubernetes/overlays/prod` renders without errors,
but the output still has every `REPLACE_ME_*` placeholder from `../../base`
unresolved, and no `jwt-secret` — applying it to a real cluster would leave
`auth-service`/`api-gateway` unable to start (fail-closed, by design — see
base's `README.md`) and every other service pointed at literal hostnames
like `REPLACE_ME_DATABASE_URL_FINANCE`.

## What this scaffold already has

- `edp-prod` namespace.
- Replica count bumped to 3 per service (not load-tested — just a starting
  point above staging's 2).

## What's still needed before this can actually be applied

Same list as `../staging/README.md` (real infra endpoints, `jwt-secret` via
a real secret manager rather than a literal, a real image registry, Ingress
host/class) — everything there applies here too, with production-grade
expectations on top:

- **Secrets must come from a real secret manager** (Vault, AWS/GCP/Azure
  Secrets Manager via External Secrets Operator, Sealed Secrets, etc.), not
  a plain `secretGenerator` literal committed anywhere, even to a private
  repo — that's acceptable for `../dev` (obviously fake, local-only value)
  but not here.
- `JWT_SECRET` must be different from staging's.
- Resource requests/limits in `../../base` (`50m`/`64Mi` request,
  `250m`/`256Mi` limit per Go service) are untuned defaults — revisit once
  there's real production load to size against, likely also needs a
  `PodDisruptionBudget` and `HorizontalPodAutoscaler` that don't exist
  anywhere in this repo yet.
- Whether Postgres/Kafka/Redis/ClickHouse run in-cluster or as managed
  services outside the cluster is a real decision that hasn't been made —
  a managed Postgres is the more common production choice, which is why
  base assumes external rather than a StatefulSet.

None of the above can be filled in honestly without real production
infrastructure existing first — this scaffold is deliberately left this
thin rather than inventing plausible-looking values for infrastructure
that doesn't exist.
