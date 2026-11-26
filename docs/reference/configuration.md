# Configuration reference

You configure every metric in an AnalysisTemplate that uses this plugin under the plugin key `argoproj-labs/rollouts-plugin-metric-datadog`. Each metric entry must have exactly one of the three source blocks (`metrics`, `monitor`, `slo`).

## Full schema

```yaml
provider:
  plugin:
    argoproj-labs/rollouts-plugin-metric-datadog:
      # --- Connection ---
      site: "datadoghq.com"          # Datadog site (see Site Values below). Default: datadoghq.com
      address: ""                    # Full URL override (https://...). Wins over site when set.
      secretRef:
        name: "my-dd-secret"         # Name of the Kubernetes Secret containing api-key/app-key
        namespaced: false            # If true, read from the AnalysisRun's namespace instead of the controller's
      timeoutSeconds: 30             # Per-measurement HTTP timeout. Default: 30s. See Timeout note below.
      onError: error                 # Phase when Datadog itself fails: error | inconclusive | pass. Default: error

      # --- Shared tag filter ---
      tags: ["service:my-service", "env:production"]

      # --- Rate limiting (optional) ---
      # NOTE: rateLimit.enabled, budgetFraction, maxConcurrent, and buckets are
      # PHASE-1-RESERVED and not yet honored. The rate limiter runs with fixed
      # built-in defaults (10 rps default bucket, 16 max concurrent). These
      # fields are accepted without error but have no effect in Phase 1.
      rateLimit:
        enabled: true                # RESERVED (Phase 1) — not yet wired; limiter always runs at built-in defaults
        budgetFraction: 0.5          # RESERVED (Phase 1) — ignored
        maxConcurrent: 16            # RESERVED (Phase 1) — ignored; built-in default is 16
        buckets:                     # RESERVED (Phase 1) — ignored
          metrics: {rps: 5.0}

      # --- Caching (optional) ---
      # NOTE: cache.ttl is PHASE-1-RESERVED. The shared cache always uses a
      # fixed 30s TTL. Setting cache.enabled: false skips reading a fresh value
      # from the TTL window, but the shared cache still coalesces concurrent
      # identical queries — it does not fully disable caching.
      cache:
        enabled: true                # Live: false skips fresh-value TTL read; cache still coalesces concurrent queries
        ttl: "30s"                   # RESERVED (Phase 1) — ignored; cache always uses 30s TTL

      # --- SDK retry (optional) ---
      retry:
        maxRetries: 3                # Live: SDK-level retries on 429/5xx. Default: 3

      # --- Source block (exactly one required) ---
      metrics: ...                   # See Metrics Source below
      monitor: ...                   # See Monitor Source below
      slo: ...                       # See SLO Source below
```

## Credential resolution order

The plugin resolves credentials in this order. The first match wins.

1. `secretRef.name`: the plugin reads the named Kubernetes Secret. If `namespaced: false` (default), the plugin reads the Secret from the controller's namespace. If `namespaced: true`, the plugin reads it from the AnalysisRun's namespace.
2. Environment variables: `DD_API_KEY` and `DD_APP_KEY`. Both are required. `DD_ADDRESS` is optional.
3. The `datadog` Secret in the controller namespace: a Secret with the literal name `datadog` in the namespace that the controller runs in. The plugin reads it automatically, with no additional configuration.

All Secrets must have the `api-key` and `app-key` keys. An optional `address` key overrides the endpoint URL.

CAUTION: Treat "can author AnalysisTemplates in namespace N" as "can read all Secrets in N". The `secretRef.name` value is unrestricted, so with `namespaced: true` anyone who can author AnalysisTemplates or AnalysisRuns in a namespace can make the controller read any Secret there and send its `api-key` and `app-key` to Datadog. To limit this, restrict Secret names by naming convention, label selector, or an admission policy (for example OPA or Kyverno) that rejects unknown `secretRef.name` values. See [install.md](../how-to/install.md#rbac) for details.

## Site values

The `site` field controls the Datadog regional endpoint. These are the common values:

| Site | Endpoint |
|---|---|
| `datadoghq.com` (default) | US1 |
| `us3.datadoghq.com` | US3 |
| `us5.datadoghq.com` | US5 |
| `datadoghq.eu` | EU1 |
| `ap1.datadoghq.com` | AP1 |
| `ddog-gov.com` | US1-FED |

To route to a custom proxy or a local test server, use `address` instead of `site`.

## Timeout and retry

`timeoutSeconds` sets the deadline for each measurement. The default is 30 seconds.

The SDK retries 429 and 5xx responses up to `retry.maxRetries` times. On a 429 response, it waits for the time in the `X-RateLimit-Reset` header. The SDK stops retrying after four fifths of `timeoutSeconds`, which is 24 seconds at the default. To size these values, see [Tune timeouts for rate limits](../how-to/tune-rate-limits.md).

## Datadog errors (onError)

`onError` sets the measurement phase when Datadog itself fails. A Datadog failure is a 5xx response, a 429 response after the SDK retries, a timeout, or a dropped connection.

| Value | Phase | Rollout effect |
|---|---|---|
| `error` (default) | `Error` | Counts toward `consecutiveErrorLimit` (default 4). Then the rollout aborts. |
| `inconclusive` | `Inconclusive` | Pauses the rollout until a person promotes or aborts it. |
| `pass` | `Successful` | The step continues. |

For all three values, the measurement keeps the error text in `message` and sets `metadata.error_class` to `datadog`. An operator can then see that a pass was not a real pass.

Setup errors always give `Error`, whatever the value of `onError`. A setup error is a bad configuration, missing credentials, or a 400, 401, 403, or 404 response. A DNS failure, a TLS failure, or a refused connection is also a setup error, because it usually means a wrong `address`. If a condition cannot evaluate the result, the phase is also `Error`.

Two Argo Rollouts defaults change the effect of `onError`:

- `inconclusiveLimit` defaults to 0. With `onError: inconclusive`, one Datadog failure pauses the rollout. To allow more failures, raise `inconclusiveLimit`.
- If a metric has `count: 1`, `onError: pass` lets one Datadog failure pass the whole analysis.

## Metrics source

```yaml
metrics:
  apiVersion: v2          # v1 or v2. Default: v2
  query: "..."            # Single DQL query (v1 or v2). Mutually exclusive with queries.
  queries:                # Named queries for multi-formula (v2 only).
    a: "sum:trace.http.request.hits{*}.as_count()"
    b: "sum:trace.http.request.errors{*}.as_count()"
  formula: "(a - b) / a"  # Required when queries has more than one entry.
  aggregator: "last"      # v2 aggregator. One of: avg, min, max, sum, last. Default: last
  interval: "5m"          # Query window width. Default: 5m. Supports Go durations and "Nd" for days.
```

The plugin merges the tags in `tags` into the query's `{...}` scope brace. If the query has no scope brace, the plugin appends the tags. For `query`, the merge applies to the raw string. For `queries`, the merge applies to each named query independently.

If the query returns no data, the result is `nil`. Otherwise, the result is a scalar `float64`. Use `result` in conditions.

```yaml
successCondition: "result >= 0.95"
```

### Time window semantics

`interval` defines the width of the `[now - interval, now]` window that the plugin sends to the API. If you omit it, the default is 5 minutes. For v1, the window bounds are seconds-precision Unix timestamps. For v2, they are milliseconds-precision Unix timestamps. The plugin computes the window at query time and does not cache it.

## Monitor source

```yaml
monitor:
  query: "muted:false"    # Additional search terms (ANDed with tags). Optional.
  id: 12345678            # Integer monitor ID. Switches to by-id mode (tags not allowed with id).
```

In search mode (no `id`), the plugin calls `SearchMonitorGroups` with a query built from `tags` joined with `monitor.query`. The result is the `SearchMonitorGroupsResponse` marshalled to a map, without the `counts.tag` facet.

In by-id mode (`id` set), the plugin calls `GetMonitor`. The result is the Monitor object marshalled to a map. In this mode, tags must be empty.

In search mode, the result shape is a `map[string]interface{}` that matches the Datadog API JSON. The useful path for status rollup is `result.counts.status`, a list of `{name, count}` objects.

```yaml
failureCondition: "result.counts.status != nil && any(result.counts.status, {.name == 'Alert' && .count > 0})"
successCondition: "result.counts.status == nil || any(result.counts.status, {.name != 'Alert'})"
```

## SLO source

```yaml
slo:
  query: ""               # Additional search terms (ANDed with tags). Optional.
  id: "abc123def456"      # String SLO ID. Switches to by-id mode (tags not allowed with id).
  interval: "7d"          # History window for by-id mode. Default: 7d. Supports "Nd" for days.
```

In search mode (no `id`), the plugin calls `SearchSLO` with a query built from `tags` joined with `slo.query`. The result is `{slos: [...], facets: {...}}`. Each element of `slos` is the SLO attributes object.

In by-id mode (`id` set), the plugin calls `GetSLOHistory` over the configured `interval`. The result is the scalar SLI value (`float64`). In this mode, tags must be empty.

Result shape in search mode:

```yaml
failureCondition: "any(result.slos, {any(.overall_status, {.state == 'breached'})})"
successCondition: "all(result.slos, {all(.overall_status, {.state != 'breached'})})"
```

In by-id mode, the result shape is a scalar `float64` SLI value.

```yaml
successCondition: "result >= 99.9"
```

## Result shapes

For the `result` type of each source and the metadata keys of each measurement, see [Result shapes and measurement metadata](results.md).

## Search warnings

If a monitor or SLO source is in search mode and has neither a `service:` tag nor a `query`, the plugin logs a warning. For the reason, see [About rate limiting and caching](../explanation/rate-limiting.md#why-search-metrics-need-a-service-tag).

## Rate limit and cache defaults

| Field | Default | Phase 1 status |
|---|---|---|
| `rateLimit.enabled` | `true` | RESERVED. Not yet wired. |
| `rateLimit.budgetFraction` | `0.5` (applied by the limiter at runtime, not by configuration defaulting) | RESERVED. Ignored. |
| `rateLimit.maxConcurrent` | `16` (applied by the limiter at runtime, not by configuration defaulting) | RESERVED. Ignored. |
| `rateLimit.buckets` | None | RESERVED. Ignored. |
| `cache.enabled` | `true` | Live. `false` skips the fresh-value TTL read. The cache still coalesces concurrent identical queries. |
| `cache.ttl` | `30s` | RESERVED. Ignored. The cache always uses a 30s TTL. |
| `retry.maxRetries` | `3` | Live. Wired to the SDK retry count. |
| `timeoutSeconds` | `30` | Live |
| `metrics.apiVersion` | `v2` | Live |
| `metrics.interval` | `5m` | Live |
| `slo.interval` (by-id) | `7d` | Live |

In Phase 1, the plugin accepts `rateLimit.*` and `cache.ttl` without error and ignores them. The rate limiter uses a default bucket of 10 requests per second and at most 16 requests in flight. The cache TTL is 30 seconds. For how the limiter and the cache work, see [About rate limiting and caching](../explanation/rate-limiting.md).
