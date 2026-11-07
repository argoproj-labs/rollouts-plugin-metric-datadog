# Argo Rollouts Datadog metric plugin: design

- Date: 2026-06-18
- Status: design, approved for spec review
- Upstream repository: `github.com/argoproj-labs/rollouts-plugin-metric-datadog`

## 1. Summary

This design describes one Argo Rollouts metric plugin, which is a standalone Go binary. The plugin lets AnalysisTemplates query Datadog across five data sources: three in Phase 1 and two in Phase 2 (see §13):

- `metrics`: scalar metric queries, with parity with the built-in Datadog provider (Phase 1)
- `monitor`: monitor (group) search by tags, or one monitor by ID (Phase 1)
- `slo`: SLO search by tags, or one SLO by ID (Phase 1)
- `apm`: APM/span analytics aggregation (Phase 2)
- `logs`: log analytics aggregation (Phase 2)

The plugin extends Datadog analysis beyond the metrics-only built-in provider to monitors, SLOs, APM traces, and logs. It has first-class tag-based filtering across every source. As a result, canary/blue-green analysis can ask questions such as *"are any monitors for `service:checkout env:production` alerting?"* or *"is any matching SLO breached?"*. It does this without hand-built HTTP jobs.

## 2. Background and motivation

### 2.1 The built-in Datadog provider (what exists today)
Argo Rollouts ships a built-in Datadog metric provider (`metricproviders/datadog`). It is metrics-only. It issues a scalar query (`/api/v2/query/scalar` for v2, `/api/v1/query` for v1), parses a single value, and evaluates it against `successCondition`/`failureCondition`. It has no concept of monitors, SLOs, APM spans, or logs.

### 2.2 The current workaround
Today, monitor checks use the generic `web` provider of Argo against the Monitor Group Search API, for example:

```yaml
provider:
  web:
    url: "https://api.datadoghq.com/api/v1/monitor/groups/search?query=muted:false%20service:{{ args.monitor-analysis-service-name }}{{ args.monitor-analysis-extra-tags }}"
    headers:
      - { key: DD-APPLICATION-KEY, value: "{{ args.app-key }}" }
      - { key: DD-API-KEY,         value: "{{ args.api-key }}" }
successCondition: result.counts.status == nil || any(result.counts.status, {.name != "Alert"})
failureCondition: result.counts.status != nil && any(result.counts.status, {.name == "Alert" && .count > 0})
```

This plugin removes these limitations:
- Manual URI-encoding of tags. The chart must pre-encode a space-separated tag string and thread it through args. The plugin takes a structured `tags: [...]` list and builds the query itself.
- Credentials leak into the CR. `api-key`/`app-key` go through `args` into request headers, so they materialize in the AnalysisRun/Rollout objects. The plugin resolves the secret in-process, and keys never enter the CRs.
- Monitors only. For SLOs, APM, and logs, you must hand-roll the same `web`-per-endpoint pattern again. The plugin unifies all five.

### 2.3 Prior art
No dedicated community Datadog metric plugin exists. The canonical template is `argoproj-labs/rollouts-plugin-metric-sample-prometheus`. Today, Datadog support lives only in the metrics-only built-in provider. Monitors/SLOs/APM/logs as analysis sources are new functionality.

### 2.4 Alternatives considered
The alternative was to extend the built-in `metricproviders/datadog` provider upstream instead of a standalone plugin. We rejected it. New capabilities come as standalone plugins because this is safer and more flexible. The plugin ships and versions independently of the controller, so no argo-rollouts release or controller upgrade is necessary to deliver or fix a source. The plugin also isolates the blast radius from the in-tree provider.

We accept these costs: a separate repository to own, per-arch binary builds/releases, binary distribution and verification, and tracking the argo-rollouts module version that the binary builds against (see §4.4).

## 3. Goals and non-goals

### Goals
- One plugin binary that covers all five sources, selectable per metric.
- Tag-based filtering as a first-class, shared concept across all sources.
- Monitor behavior that is drop-in-compatible with the existing `web`-provider conditions.
- Credentials resolved securely in-process (never in CRs). The resolution order mirrors the built-in provider, so existing `datadog` secrets work unchanged.
- Multi-region/site support (US1/EU/US3/US5/AP1/gov).
- Condition semantics identical to every other Argo provider (reuse `evaluate.EvaluateResult`).
- Rate-limit safety at fleet scale. Many rollouts share one Datadog org budget. Shared client-side limiting, coalescing, and caching stop them from throttling each other or failing canaries on `429`s (see §15).

### Non-goals
- Async/long-running queries. All sources are synchronous (one request per `Run()`). They use aggregation endpoints instead of raw event search plus polling.
- Multiple data sources inside a single metric. You compose sources with multiple `metrics[]` entries (see §10). One source per metric.
- Re-implementing arbitrary parts of the Datadog API. The plugin uses only the endpoints that the five sources need.

## 4. Architecture

### 4.1 One binary, internal `DataSource` strategy
A single executable implements the Rollouts `MetricProviderPlugin` RPC interface once. Inside, a small `DataSource` interface has one implementation per source. The configuration sub-block that is present selects the implementation:

```go
type DataSource interface {
    // Query fetches and returns a value (scalar float, string, or structured
    // map) plus metadata describing what was queried. The caller feeds the
    // value to evaluate.EvaluateResult and maps errors to the Measurement.
    Query(ctx context.Context, run *v1alpha1.AnalysisRun, metric v1alpha1.Metric,
        cfg *Config) (value interface{}, metadata map[string]string, err error)
}
```

Shared infrastructure lives once in the `Run()` wrapper: Datadog client construction, credential resolution, site mapping, tag application, the `evaluate.EvaluateResult` call, and error→`Measurement` mapping. Each source is a focused, independently testable unit. New sources (apm, logs in phase 2) are new `DataSource` implementations with zero churn to the core.

### 4.2 Datadog access via the official Go SDK
The plugin uses `github.com/DataDog/datadog-api-client-go/v2` (sub-packages `datadogV1`, `datadogV2`, `datadog`). The SDK covers every endpoint with typed request/response, one auth+site path, and built-in pagination. As a result, we do not hand-roll structs and parsing across ~7 heterogeneous endpoints.

| Source | SDK call | Endpoint |
|---|---|---|
| metrics | `datadogV2.MetricsApi.QueryScalarData` (v2) / v1 query | `/api/v2/query/scalar` |
| monitor (search) | `datadogV1.MonitorsApi.SearchMonitorGroups` | `/api/v1/monitor/groups/search` |
| monitor (by id) | `datadogV1.MonitorsApi.GetMonitor` | `/api/v1/monitor/{id}` |
| slo (search) | `datadogV1.ServiceLevelObjectivesApi.SearchSLO` | `/api/v1/slo/search` |
| slo (by id) | `datadogV1.ServiceLevelObjectivesApi.GetSLOHistory` | `/api/v1/slo/{id}/history` |
| apm | `datadogV2.SpansApi.AggregateSpans` | `/api/v2/spans/analytics/aggregate` |
| logs | `datadogV2.LogsApi.AggregateLogs` | `/api/v2/logs/analytics/aggregate` |

### 4.3 Plugin mechanism (fixed by Argo Rollouts)
See the [Argo Rollouts metric plugin docs](https://argo-rollouts.readthedocs.io/en/stable/analysis/plugins/).

- Transport. HashiCorp go-plugin over `net/rpc`/gob (not gRPC). The controller launches the plugin as a managed, long-lived child process and talks over a local socket. Plugins must be Go.
- Handshake. These values must match exactly in `main.go`:
  - `ProtocolVersion: 1`
  - `MagicCookieKey: "ARGO_ROLLOUTS_RPC_PLUGIN"`
  - `MagicCookieValue: "metricprovider"`
  - Plugin map key that the controller dispenses: `"RpcMetricProviderPlugin"`
- Interface implemented. `InitPlugin`, `Run`, `Resume`, `Terminate`, `GarbageCollect`, `Type`, `GetMetadata`. Because the plugin is synchronous, `Resume`/`Terminate` return the measurement unchanged, and `GarbageCollect` returns an empty `RpcError{}`. All work happens in `Run`.
- Install and discovery. The `argo-rollouts-config` ConfigMap of the controller lists the plugin under `metricProviderPlugins`:
  ```yaml
  data:
    metricProviderPlugins: |-
      - name: "argoproj-labs/rollouts-plugin-metric-datadog"
        location: "file://./plugin-bin/argoproj-labs/rollouts-plugin-metric-datadog"
        # or location: "https://github.com/argoproj-labs/rollouts-plugin-metric-datadog/releases/download/vX/...-linux-amd64"
        #    sha256: "<hex>"
  ```
  The controller resolves the binary under `<cwd>/plugin-bin/<namespace>/<name>`. The documented install method is an `https://` location with `sha256` verification at controller startup.
- The plugin name in the ConfigMap must equal the map key in `provider.plugin.<name>` in AnalysisTemplates. The canonical name is `argoproj-labs/rollouts-plugin-metric-datadog`. Operators can alias it.

### 4.4 Argo Rollouts version compatibility
The plugin imports argo-rollouts packages (`v1alpha1`, `utils/evaluate`, `utils/metric`, RPC types) as a pinned Go-module dependency. It runs as a child process of the controller, and the controller can be a different version. The `net/rpc`/gob handshake (`ProtocolVersion: 1`) and the `Metric`/`Measurement` struct shapes are the parts that must stay compatible. A binary built against a drifted module can mis-marshal or fail at `Run()`.

Policy: each release states the supported argo-rollouts controller version range. The argo-rollouts version in the plugin `go.mod` tracks the controller that it runs in. A `ProtocolVersion` mismatch shows as a handshake failure in the controller log at plugin startup.

## 5. Configuration schema

The configuration is schemaless from the perspective of the CRD. The plugin receives `metric.Provider.Plugin["argoproj-labs/rollouts-plugin-metric-datadog"]` as raw JSON and unmarshals it into its own struct.

### 5.1 Top-level shape (per-source nested blocks)

```yaml
provider:
  plugin:
    argoproj-labs/rollouts-plugin-metric-datadog:
      # --- shared connection (all optional; resolved via the credential chain) ---
      site: "datadoghq.com"          # or datadoghq.eu / us3.datadoghq.com / us5.datadoghq.com / ap1.datadoghq.com / gov
      address: "https://api.datadoghq.com"   # optional full-URL override (built-in compat)
      secretRef:
        name: my-datadog-secret
        namespaced: false
      timeoutSeconds: 30             # optional; default 30
      # rateLimit / cache / retry: fleet-scale controls (all optional) — see §15.4

      # --- shared tag filter, applied per source (see §7) ---
      tags:
        - "service:my-cool-service"
        - "env:production"

      # --- EXACTLY ONE of the five source blocks (validated) ---
      metrics: { ... }
      monitor: { ... }
      slo:     { ... }
      apm:     { ... }
      logs:    { ... }
```

The plugin applies these validation rules:
- Exactly one of `metrics`/`monitor`/`slo`/`apm`/`logs` must be present. `apm`/`logs` are Phase 2 (see §13).
- For `metrics`, exactly one of `query`/`queries` must be present. `formula` requires `queries`. `>1` query requires `formula`. `aggregator` is valid only for v2. `apiVersion` ∈ {v1, v2} (default v2).
- If `tags` are combined with a by-id mode (`monitor.id`/`slo.id`), validation rejects them (§7.4).
- `secretRef.namespaced: true` requires a non-empty `name`. If the name is empty, validation rejects the configuration, because there is no fallback.
- `site` and `address` are mutually exclusive ways to target a region. If both are set, `address` wins.

### 5.2 Per-source blocks

#### metrics (built-in parity)
```yaml
metrics:
  apiVersion: v2                 # v1 | v2 (default v2)
  query: "avg:trace.http.request.duration{*}"   # v1, or single v2 query
  queries:                       # v2 multi-query
    a: "sum:trace.http.request.hits{*}.as_count()"
    b: "sum:trace.http.request.errors{*}.as_count()"
  formula: "(a - b) / a"         # v2 only; required when >1 query
  aggregator: "last"             # v2 only
  interval: "5m"                 # Datadog lookback window (default 5m)
```
`result` = scalar float. Example: `successCondition: "result >= 0.95"`. In the SDK, `aggregator` is a property of each scalar query, not of the formula. The plugin applies the single block-level value to every (sub-)query in a formula.

#### monitor
```yaml
monitor:
  mode: search                   # default; or set `id:` for by-ID mode
  query: "muted:false"           # optional extra query terms ANDed with shared tags
  # id: 1234567                  # by-ID mode → GetMonitor
```
- Search mode (default). `SearchMonitorGroups` with `query = <tags> <extra query>`. `result` = the group-search response (`counts.status` `[{name, count}]`, `groups`, `metadata`). The shape is unchanged, so existing conditions work:
  ```yaml
  failureCondition: "any(result.counts.status, {.name == 'Alert' && .count > 0})"
  successCondition: "result.counts.status == nil || any(result.counts.status, {.name != 'Alert'})"
  ```
  The plugin drops one field from the response: `counts.tag`. Datadog computes that facet over the entire matched set and caps it at 1000 entries. As a result, it scales with the org, not with the query. In a measurement against a real org, it was 54KB of an 80KB measurement value. The `counts.status` field that these conditions read was 116 bytes.

  Argo Rollouts retains 10 measurements per metric. Keeping `counts.tag` cost ~800KB of AnalysisRun status per metric. It can push a multi-metric template past the 1.5MB request limit of etcd. `counts.status`, `counts.muted`, `counts.type`, `groups` and `metadata` are unchanged.
- By-ID mode. `GetMonitor`. `result` = monitor JSON, for example `successCondition: "result.overall_state == 'OK'"`.

#### slo
```yaml
slo:
  mode: search                   # default; or set `id:` for by-ID mode
  query: "slo_type:metric"       # optional extra query terms ANDed with shared tags
  # id: "abcd1234..."            # by-ID mode → GetSLOHistory
  # interval: "7d"               # by-ID lookback window for history (default 7d)
  # field: sli                   # by-ID only: sli only (error-budget-remaining is not returned by GetSLOHistory — use search mode)
```
- Search mode (default). `SearchSLO` with `query = <tags> <extra query>`. Datadog returns the breach state inline for each SLO. For ergonomics, the plugin lightly flattens `result` to `{ slos: [<attributes>...], facets: <...> }`. Each element carries `overall_status` (`[{state, status, target, errorBudgetRemaining, ...}]`, `state` ∈ breached|warning|ok|no_data), `name`, `all_tags`/`service_tags`/`env_tags`/`team_tags`, `groups`, `thresholds`:
  ```yaml
  failureCondition: "any(result.slos, {any(.overall_status, {.state == 'breached'})})"
  successCondition: "all(result.slos, {all(.overall_status, {.state != 'breached'})})"
  ```
- By-ID mode. `GetSLOHistory` over `interval`. `result` = SLI attainment % float, for example `successCondition: "result >= 99.9"`. `GetSLOHistory` does not return error-budget-remaining. Use search mode, which carries `errorBudgetRemaining` inline for each SLO through `overall_status`.

#### apm
```yaml
apm:
  query: "operation_name:http.request"   # span filter; shared tags ANDed in
  aggregation: pc95                       # count|cardinality|avg|min|max|sum|median|pc75|pc90|pc95|pc98|pc99
  metric: "@duration"                     # required for measure aggregations (percentiles, avg, …); count/cardinality ignore it
  interval: "5m"                          # lookback window
```
`result` = scalar float, for example hits, p95 latency, or error rate. Example: `failureCondition: "result > 500"`. Extraction: a single unnamed `compute` with no group-by yields one bucket, and the plugin reads the value of that bucket. Zero buckets → nil (empty-result handling, §9).

#### logs
```yaml
logs:
  query: "status:error"          # log search filter; shared tags ANDed in
  aggregation: count             # count|cardinality|avg|min|max|sum|median|pc75|pc90|pc95|pc98|pc99
  metric: "@duration"            # required for measure aggregations
  indexes: ["main"]              # optional
  interval: "5m"                 # lookback window
```
`result` = scalar float, for example the error-log count. Example: `failureCondition: "result > 10"`. Extraction uses the same single-bucket rule as apm above. `indexes` maps to the SDK request `Filter.Indexes`.

### 5.3 Time windows (`interval`)
`interval` is a Go duration string, for example `5m` or `7d`. At call time, the plugin computes `to = now` and `from = now - interval`, and converts them to the unit that each endpoint expects. The v2 scalar, spans, and logs endpoints take epoch milliseconds. `GetSLOHistory` takes epoch seconds. Defaults: 5m for metrics/apm/logs, 7d for slo by-id.

## 6. Result and verdict model

The core principle is that the plugin never hard-codes pass/fail. Each source resolves to a value (scalar, string, or structured map). `Run()` calls `evaluate.EvaluateResult(value, metric, logCtx)`, the same evaluator that every Argo provider uses. As a result, `successCondition`/`failureCondition` semantics are identical. `evaluate.EvaluateResult` accepts `interface{}`, which is how the `web` provider already evaluates structured JSON. This is what makes the monitor/SLO structured results work.

The Datadog SDK returns typed Go structs, but the `web` provider feeds raw `map[string]interface{}`. Each structured source marshals its typed SDK response to JSON and re-unmarshals it into `map[string]interface{}` before `EvaluateResult`. This keeps existing `web`-provider conditions (for example `result.counts.status`) working unchanged. A §12 test proves that the field names match the raw Datadog API JSON that the conditions traverse. Without this conversion, a migrated condition can silently stop matching.

Result shapes by source:
- metrics, apm, logs → scalar float
- monitor (by-id) → monitor object (`result.overall_state`)
- monitor (search) → group-search response (for example `result.counts.status`)
- slo (by-id) → scalar float (SLI %)
- slo (search) → `{ slos: [...], facets: {...} }`

## 7. Tag filtering (shared `tags`)

`tags` is a top-level list of raw Datadog tag terms, AND-combined. It is a list of strings (not a map), so negation (`!env:staging`) and wildcards (`service:web-*`) work. The plugin applies tags differently per source, because the sources differ structurally.

### 7.1 Query-string sources (apm, logs)
The plugin ANDs tags into the search `filter.query` string: `<user query> service:my-cool-service env:production`.

### 7.2 Metric-scope source (metrics)
The metric scope lives inside the query braces. The plugin merges tags into the `{…}` scope of each (sub-)query in a v2 formula. The scope brace is the first `{…}` immediately after a metric name. The plugin parses a trailing `by {…}` grouping clause (and any `.as_count()`/function suffix) separately and preserves it. It rewrites each sub-query in a formula independently. The merge rules are:
- `metric{*}` or `metric{}` → `metric{service:my-cool-service,env:production}`
- `metric{existing:tag}` → `metric{existing:tag,service:my-cool-service,env:production}`
- The plugin preserves grouping (`by {…}`).
- If a query has no scope brace, the plugin appends one after the metric name.
- If a key is present in both the query scope and shared `tags` (a same-key collision), the plugin keeps both terms (ANDed). It does not dedupe or override, so the condition author is responsible for non-contradictory keys.

### 7.3 Search sources (monitor, slo)
The plugin joins tags into the search `query`, for example `service:my-cool-service env:production`. It combines them with the optional `query` terms of each source, for example `muted:false`.

### 7.4 By-ID modes
When `monitor.id`/`slo.id` is set, the scope of the object is fixed, and `tags` do not apply. If tags are combined with a by-ID mode, the plugin rejects them at validation time with a configuration error and a clear message. Tag-driven group/SLI selection within a single object is out of scope. Use search mode for tag filtering.

## 8. Credentials, RBAC, region

### 8.1 Resolution order (mirrors the built-in provider)
1. Per-metric `secretRef` (`name`, `namespaced`). If `namespaced: true`, the plugin reads from the namespace of the AnalysisRun. Otherwise, it reads from the controller namespace.
2. Environment variables: `DD_API_KEY`, `DD_APP_KEY`, `DD_ADDRESS`.
3. A secret literally named `datadog` in the controller namespace.

Secret keys: `api-key`, `app-key`, optional `address`. An existing `datadog` secret with `api-key`/`app-key` drops straight into step 3. Otherwise, the top-level `site` configuration field selects the region (§5.1, §8.3). The built-in provider has no `DD_SITE` env var and no `site` secret key.

### 8.2 RBAC
The plugin runs as a child process of the controller and inherits its ServiceAccount. It uses in-cluster configuration to read secrets. The default argo-rollouts install grants the controller SA a cluster-scoped `secrets` get/list/watch ClusterRole. That ClusterRole already covers the cross-namespace case for `namespaced: true`. As a result, a default install requires no new RBAC. If an install narrowed the SA to namespace-scoped secret access, it must add a grant for any namespace that a `namespaced: true` secretRef targets.

### 8.3 Region / site
Prefer `site`, mapped to the SDK `datadog.ContextServerVariables{"site": …}`. The plugin accepts `address` (full URL) for built-in compatibility, and `address` overrides the derived server. The plugin sets auth through `datadog.ContextAPIKeys` with `apiKeyAuth` and `appKeyAuth`.

## 9. Error handling and empty results

- Any SDK error, `401`/`403` auth failure, or parse error → `metricutil.MarkMeasurementError(m, err)` (`Phase: Error` + message). This returns the measurement by value, so you must capture the result (`m = metricutil.MarkMeasurementError(m, err)`). The `consecutiveErrorLimit` of the controller (default 4) governs transient-error tolerance.
- The HTTP timeout default is 30s, because aggregations are slower than point queries. You can override it with the top-level `timeoutSeconds`.
- Aggregation latency/throttling (apm/logs). Wide windows, high-cardinality filters, or Datadog rate limiting (`429`) can make a synchronous `AggregateSpans`/`AggregateLogs` call exceed the timeout. The timeout maps to `Phase: Error` and consumes `consecutiveErrorLimit`. As a result, a canary fails on Datadog latency, not on a real regression. §15 gives the defenses in detail: a shared client-side rate limiter, header-adaptive control, request coalescing plus a short-TTL cache, header-aware SDK retry, and last-known-good degradation. The defenses never produce `Failed`.
- Empty results:
  - Structured sources (monitor/slo) pass the real (possibly empty) response to expr, so conditions like `result.counts.status == nil || …` keep working. Emptiness is the concern of the condition author.
  - If the result is empty, scalar sources (metrics/apm/logs) pass a nil value. Then `evaluate.EvaluateResult` surfaces a clear "no value" error (`Phase: Error`), identical to the built-in provider.
- Observability. `Measurement.Metadata` (and `GetMetadata`) records the resolved query/endpoint and applied tags. As a result, operators see exactly what was queried from the AnalysisRun status.

## 10. Multi-source composition

Each metric has one source, and validation enforces this. To combine sources, list multiple `metrics[]` in one AnalysisTemplate. When either a monitor alert or an SLO breach must fail the run, use this pattern. The engine runs each metric independently. If any metric fails, the AnalysisRun fails, and it succeeds only if all pass. Each signal gets its own `interval`, `failureLimit`, and `consecutiveErrorLimit`.

```yaml
spec:
  metrics:
    - name: monitors
      interval: 1m
      failureLimit: 2
      provider:
        plugin:
          argoproj-labs/rollouts-plugin-metric-datadog:
            tags: ["service:my-cool-service", "env:production"]
            monitor: {}
      failureCondition: "any(result.counts.status, {.name == 'Alert' && .count > 0})"
    - name: slos
      interval: 1m
      failureLimit: 2
      provider:
        plugin:
          argoproj-labs/rollouts-plugin-metric-datadog:
            tags: ["service:my-cool-service", "env:production"]
            slo: {}
      failureCondition: "any(result.slos, {any(.overall_status, {.state == 'breached'})})"
```

The multiple-`metrics[]` model expresses OR-of-failures cleanly (any metric that fails fails the run). But it cannot express a single cross-source AND in one expression. As a result, you must model a use case like "monitor alerting AND SLO budget < 10%" as two independent signals (OR semantics). One combined condition is not possible. Genuine cross-source AND within a single expression is explicitly out of scope (see §3 non-goals).

## 11. Repository layout

Repository: `github.com/argoproj-labs/rollouts-plugin-metric-datadog`. It follows the argoproj-labs `<org>/<plugin-name>` naming convention.

```
main.go                      # go-plugin handshake + Serve (from sample)
internal/plugin/
  plugin.go                  # RpcPlugin: InitPlugin/Run/Resume/Terminate/GC/Type/GetMetadata
  config.go                  # config structs + validation
internal/datasource/
  datasource.go              # DataSource interface + dispatch by config block
  metrics.go monitor.go slo.go    # Phase 1
  apm.go logs.go                  # Phase 2
  tags.go                    # shared tag application per source
internal/datadog/
  client.go                  # SDK client construction, site mapping
  credentials.go             # credential resolver + kube client
docs/  examples/  Makefile  .github/workflows/
```

For code reuse, the plugin imports argo-rollouts as a Go module dependency. It imports `v1alpha1` types, `metricproviders/plugin/rpc`, `utils/plugin/types`, `utils/evaluate`, and `utils/metric`. It does not copy types, so condition semantics stay identical.

## 12. Testing strategy

- Each `DataSource` has unit tests against an `httptest.Server` that returns canned Datadog JSON (the SDK targets a configurable server URL). Table-driven cases cover value parsing, tag injection/merge, empty results, and error mapping.
- Tests cover configuration validation directly (exactly-one-source, metrics field rules, tags+by-id rejection).
- One `evaluate.EvaluateResult` integration test per source proves condition semantics, including the structured monitor/SLO conditions.
- A migration-parity test feeds canned `SearchMonitorGroups` JSON through the struct→`map[string]interface{}` conversion (§6). It asserts that the existing `web`-provider conditions (for example `result.counts.status`) still match. This guards the drop-in goal against SDK field-name drift.
- CI (GitHub Actions) builds per-arch binaries, runs tests, and publishes artifacts plus `sha256`.

## 13. Phasing

Phase 1 covers scaffolding and the synchronous lookup sources:
- The go-plugin handshake, RPC implementation, configuration and validation, SDK client construction, credential resolver, and the `DataSource` interface and dispatch.
- `metrics` (built-in parity), `monitor` (tag search + by-id, the migration target), `slo` (tag search + by-id).
- Rate-limit and scale core (§15): shared limiter, header-adaptive control, coalescing plus short-TTL cache, header-aware SDK retry, last-known-good degradation.
- Docs and examples, including the `web`-provider → plugin migration.

Phase 2 covers the aggregation sources:
- `apm` (`AggregateSpans`) and `logs` (`AggregateLogs`) as two new `DataSource` implementations, with no core churn.
- Cache/interval tuning for apm/logs, and usage guidance for the much stricter analytics budgets (§15).
- Expanded examples.

## 14. Distribution and migration

- GitHub releases with per-arch binaries plus `sha256`. The controller `argo-rollouts-config` references the release binary through `https://` with sha verification.
- Migration doc: from the `cluster-datadog-monitor.yaml` (`web` provider, manual tag encoding, keys-in-args) to the plugin form (structured `tags`, in-process credential resolution, same conditions).

## 15. Rate limiting and scale resilience

### 15.1 The problem
Datadog enforces API rate limits per organization (also per API key and per user). As a result, every rollout in the cluster shares one budget. It also shares that budget with all other Datadog API consumers in the org (Terraform, dashboards, automation). At fleet scale (for example 200 concurrent rollouts), the plugin can both exhaust that budget and be starved by other consumers.

The failure that matters is this: sustained `429`s become `Phase: Error`. After `consecutiveErrorLimit` (default 4) consecutive errors, the controller aborts the rollout on Datadog throttling, not on a real regression. A throttle must never map to `Failed`. `failureLimit` defaults to 0, so a single `Failed` aborts immediately.

The known limits follow. They are confirmed where a source is cited. The plugin discovers the others at runtime (§15.3). See [Datadog API rate limits](https://docs.datadoghq.com/api/latest/rate-limits/):
- `/api/v1/query`: 1,600 req / 60s (~26/s) per org. A verbatim Datadog throttle error in [kedacore/keda#5521](https://github.com/kedacore/keda/issues/5521) confirms this.
- `v2/query/scalar`, monitor, and SLO endpoints: not publicly documented.
- apm (`spans/analytics/aggregate`) and logs (`logs/analytics/aggregate`): only secondary sources report ~300 req/hour (~0.08/s), and this is unconfirmed. If this is accurate, these endpoints are ~300× stricter. ~5 rollouts that poll logs every 60s saturate the whole org budget, and 200 rollouts are ~40× over.

There are two scale effects:
- Burst alignment. The controller adds no jitter and requeues each metric exactly `interval` after the previous one finishes. Canaries that start together in a deploy wave fire aligned. When steady-state QPS is safe, N rollouts still produce an N-call spike within ~1s.
- Steady-state aggregate. 200 rollouts × M metrics ÷ `interval`, summed per rate-limit bucket. The analytics endpoints are the binding constraint.

### 15.2 Why a client-side defense works here
The plugin is a singleton process per controller (§4.3). Leader election is on by default, so exactly one controller is active cluster-wide. As a result, a single in-process layer sees 100% of the Datadog traffic of the cluster and can coordinate globally. The controller calls `Run()` from up to 30 concurrent workers (plus per-run metric goroutines), so this shared state must be goroutine-safe.

### 15.3 Defense layers (shared `Run()` wrapper, Phase 1 core)
1. Shared token-bucket limiter. There is one bucket per Datadog rate-limit group, keyed by the `X-RateLimit-Name` response header. Each bucket is sized to a configurable fraction of the budget. `Run()` blocks on the limiter before it issues a request, and this converts bursts into a sustained sub-ceiling rate. The limiter is goroutine-safe (`golang.org/x/time/rate`).
2. Adaptive control from headers. Every response carries `X-RateLimit-Limit/Remaining/Reset/Name`. The limiter learns the (often undocumented) ceiling from `X-RateLimit-Limit` and slows as `Remaining`→0. On `429`, it waits `X-RateLimit-Reset` seconds. Datadog sends no `Retry-After`.
3. Coalescing and short-TTL cache. Singleflight collapses identical concurrent queries. The plugin caches results for a short TTL (≤ the metric `interval`), keyed by the fully-resolved query + tags + window + site. Many rollouts that share the same `service:`/`env:` tags then cost one upstream call instead of N. This is what makes apm/logs survivable at scale. Teams that need strictly per-call freshness can opt out per metric.
4. Concurrency semaphore. It caps in-flight outbound calls, regardless of how many `Run()`s arrive at once.
5. SDK retry enabled. `RetryConfiguration.EnableRetry = true` (the SDK default is off), so the SDK retries `429`/5xx and honors `X-Ratelimit-Reset`. `MaxRetries`/`HTTPRetryTimeout` are bounded below the plugin `timeoutSeconds`.
6. Safe degradation (never `Failed`). Under pressure, the order is limiter wait → coalesced/cached value → SDK retry. If the request is still throttled within the `Run()` deadline, the plugin serves the last-known-good cached value when one exists within `maxStaleness`. Otherwise, it returns `Error`, bounded by `consecutiveErrorLimit`, and recovers automatically on the next success. A throttle never yields `Failed`.
7. Observability. `Measurement.Metadata` records the bucket (`X-RateLimit-Name`), remaining budget, limiter wait, and whether the value was fresh, coalesced, cached, or last-good. As a result, operators see budget pressure in the AnalysisRun status.

### 15.4 Configuration surface
These fields are top-level, next to `site`/`timeoutSeconds`. All are optional and have safe defaults:
```yaml
rateLimit:
  enabled: true                 # default true
  budgetFraction: 0.5           # use at most 50% of the discovered/declared org budget
  buckets:                      # optional static ceilings used until headers are seen
    metrics_query: { rps: 13 }  # e.g. ~half of 1600/60s
  maxConcurrent: 16             # outbound in-flight cap
cache:
  enabled: true                 # default true; opt-out per metric via cache: {enabled: false}
  ttl: "30s"                    # default min(interval, 30s)
  maxStaleness: "5m"            # last-known-good ceiling for degradation
retry:
  maxRetries: 3                 # SDK retry; X-Ratelimit-Reset honored
```

### 15.5 Operator guidance
- Coordinate `budgetFraction` with the other Datadog API consumers of the org.
- Prefer longer `interval`s and shared tags for apm/logs metrics. Keep per-rollout high-frequency log/APM polling as the exception, because it does not scale.
- Request per-bucket limit increases from Datadog with the `X-RateLimit-Name` value that `Measurement.Metadata` shows.
- If possible, stagger large deploy waves. The limiter de-synchronizes bursts, but it cannot create budget that does not exist.

### 15.6 Testing
- Limiter/concurrency. Assert that issued QPS stays under the configured ceiling under N concurrent `Run()`s.
- Coalescing/cache. Assert that K identical concurrent queries produce 1 upstream call. Assert TTL, `maxStaleness`, and last-good degradation.
- 429 handling. An `httptest.Server` that returns `429` + `X-RateLimit-Reset` asserts retry timing and the Error-vs-last-good path. It also asserts that a throttle never yields `Failed`.
