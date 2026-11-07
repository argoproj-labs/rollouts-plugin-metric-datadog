# Configuration reference

You configure every metric in an AnalysisTemplate that uses this plugin under the plugin key `argoproj-labs/rollouts-plugin-metric-datadog`. Each metric entry must have exactly one of the three source blocks (`metrics`, `monitor`, `slo`).

---

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

      # --- Shared tag filter ---
      tags: ["service:my-service", "env:production"]

      # --- Rate limiting (optional) ---
      # NOTE: rateLimit.enabled, budgetFraction, maxConcurrent, and buckets are
      # PHASE-1-RESERVED and not yet honored. The rate limiter runs with fixed
      # built-in defaults (10 rps default bucket, 16 max concurrent). These
      # fields are parsed and validated but have no runtime effect in Phase 1.
      rateLimit:
        enabled: true                # RESERVED (Phase 1) — not yet wired; limiter always runs at built-in defaults
        budgetFraction: 0.5          # RESERVED (Phase 1) — parsed but ignored
        maxConcurrent: 16            # RESERVED (Phase 1) — parsed but ignored; built-in default is 16
        buckets:                     # RESERVED (Phase 1) — parsed but ignored
          metrics: {rps: 5.0}

      # --- Caching (optional) ---
      # NOTE: cache.ttl is PHASE-1-RESERVED. The shared cache always uses a
      # fixed 30s TTL. Setting cache.enabled: false skips reading a fresh value
      # from the TTL window, but the shared cache still coalesces concurrent
      # identical queries — it does not fully disable caching.
      cache:
        enabled: true                # Live: false skips fresh-value TTL read; cache still coalesces concurrent queries
        ttl: "30s"                   # RESERVED (Phase 1) — parsed but ignored; cache always uses 30s TTL

      # --- SDK retry (optional) ---
      retry:
        maxRetries: 3                # Live: SDK-level retries on 429/5xx. Default: 3

      # --- Source block (exactly one required) ---
      metrics: ...                   # See Metrics Source below
      monitor: ...                   # See Monitor Source below
      slo: ...                       # See SLO Source below
```

---

## Credential resolution order

The plugin resolves credentials in this order. The first match wins.

1. `secretRef.name`: the plugin reads the named Kubernetes Secret. If `namespaced: false` (default), the plugin reads the Secret from the controller's namespace. If `namespaced: true`, the plugin reads it from the AnalysisRun's namespace.
2. Environment variables: `DD_API_KEY` and `DD_APP_KEY`. Both are required. `DD_ADDRESS` is optional.
3. The `datadog` Secret in the controller namespace: a Secret with the literal name `datadog` in the namespace that the controller runs in. The plugin reads it automatically, with no additional configuration.

All Secrets must have the `api-key` and `app-key` keys. An optional `address` key overrides the endpoint URL.

CAUTION: Operators must treat "can author AnalysisTemplates in namespace N" as equivalent to "can read all Secrets in N". As an alternative, restrict the eligible Secret names by convention, label, or admission policy. This is the secretRef escalation risk. The `secretRef.name` value is unrestricted. With `namespaced: true`, the plugin reads from the AnalysisRun's namespace through the controller ServiceAccount. As a result, anyone who can author AnalysisTemplates or AnalysisRuns in a namespace can instruct the controller to read any Secret in that namespace. The controller then forwards the `api-key`/`app-key` of that Secret to Datadog. See [install.md](install.md#rbac) for details.

---

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

---

## Timeout and rate-limit retry

`timeoutSeconds` sets the hard deadline for each measurement. The SDK's internal retry timer is bounded to `timeout - timeout/5`, so the plugin can observe a clean context cancellation.

For `metrics` and `monitor`, a 429 response causes the SDK to honor the `X-RateLimit-Reset` header. For the `/api/v1/query` endpoint, this header can be up to ~60 seconds. The retry fires only if time remains within the SDK retry window (`timeout * 4/5`).

If you need the header-aware retry to fire before the context deadline, set `timeoutSeconds` to at least 60–90 seconds. With the default of 30s, the SDK retry window is only 24s. A Reset value near 60s will exhaust the deadline before the retry can run.

---

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

---

## Monitor source

```yaml
monitor:
  query: "muted:false"    # Additional search terms (ANDed with tags). Optional.
  id: 12345678            # Integer monitor ID. Switches to by-id mode (tags not allowed with id).
```

In search mode (no `id`), the plugin calls `SearchMonitorGroups` with a query built from `tags` joined with `monitor.query`. The result is the full `SearchMonitorGroupsResponse` marshalled to a map.

In by-id mode (`id` set), the plugin calls `GetMonitor`. The result is the Monitor object marshalled to a map. In this mode, tags must be empty.

In search mode, the result shape is a `map[string]interface{}` that matches the Datadog API JSON. The useful path for status rollup is `result.counts.status`, a list of `{name, count}` objects.

```yaml
failureCondition: "any(result.counts.status, {.name == 'Alert' && .count > 0})"
successCondition: "result.counts.status == nil || any(result.counts.status, {.name != 'Alert'})"
```

---

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

---

## Result shape table

| Source | Mode | `result` type | Example condition |
|---|---|---|---|
| `metrics` | v1 or v2 | `float64` (nil if no data) | `result >= 0.95` |
| `monitor` | search | `map` with `result.counts.status[].{name, count}` | `any(result.counts.status, {.name == 'Alert' && .count > 0})` |
| `monitor` | by-id | `map` with the full Monitor JSON | field access per Monitor schema |
| `slo` | search | `map` with `result.slos[].overall_status[].{state, ...}` | `any(result.slos, {any(.overall_status, {.state == 'breached'})})` |
| `slo` | by-id | `float64` SLI value | `result >= 99.9` |

---

## Service-tag guidance

A `service:...` tag on each metric makes the resolved query identify one gated resource. A monitor or SLO search scoped only by `env:production` (or similar broad tags) can match resources across multiple rollouts. If this occurs, the shared cache coalesces results from different services.

A monitor or SLO source can be in search mode with neither a `service:...` tag nor a per-source `query`. In this case, the plugin emits a log warning. `Config.Warnings()` surfaces this warning. The examples in this repository all use `tags: ["service:my-cool-service", "env:production"]` as the recommended baseline.

---

## Rate limit and cache defaults

| Field | Default | Phase 1 status |
|---|---|---|
| `rateLimit.enabled` | `true` | RESERVED. Not yet wired. |
| `rateLimit.budgetFraction` | `0.5` (applied by the limiter at runtime, not by configuration defaulting) | RESERVED. Parsed but ignored. |
| `rateLimit.maxConcurrent` | `16` (applied by the limiter at runtime, not by configuration defaulting) | RESERVED. Parsed but ignored. |
| `rateLimit.buckets` | None | RESERVED. Parsed but ignored. |
| `cache.enabled` | `true` | Live. `false` skips the fresh-value TTL read. The cache still coalesces concurrent identical queries. |
| `cache.ttl` | `30s` | RESERVED. Parsed but ignored. The cache always uses a 30s TTL. |
| `retry.maxRetries` | `3` | Live. Wired to the SDK retry count. |
| `timeoutSeconds` | `30` | Live |
| `metrics.apiVersion` | `v2` | Live |
| `metrics.interval` | `5m` | Live |
| `slo.interval` (by-id) | `7d` | Live |

In Phase 1, the plugin parses and stores `rateLimit.*` (enabled/budgetFraction/maxConcurrent/buckets) and `cache.ttl`, but these fields have no runtime effect. The rate limiter runs with fixed built-in defaults: 10 rps default bucket, 16 max concurrent. The cache TTL is fixed at 30s. These fields are present in the schema so that operator configurations written today will be forward-compatible when Phase 2 wires them.

All concurrent measurements in the same plugin process share the rate limiter. The rate limiter learns per-endpoint Datadog bucket names from `X-RateLimit-Name` response headers. It then adapts its token-bucket ceiling accordingly (layer 2 adaptation). The concurrency semaphore (layer 4) prevents thundering-herd bursts during large canary waves.
