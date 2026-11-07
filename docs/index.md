# rollouts-plugin-metric-datadog

An [Argo Rollouts](https://argoproj.github.io/argo-rollouts/) metric plugin that gates canary and blue-green rollouts on Datadog observability data. Phase 1 supports three sources: **metrics** (v1 and v2 scalar queries with multi-query formulas), **monitor** (group search or by-id), and **SLO** (search or by-id history). The plugin resolves Datadog credentials in-process, merges structured `tags` into queries automatically, and provides shared rate-limiting and short-TTL caching across concurrent rollout waves. Built and tested against Argo Rollouts v1.10.2.

## Supported sources (Phase 1)

| Source | Modes | Result |
|---|---|---|
| `metrics` | v1 query, v2 scalar formula | scalar `float64` |
| `monitor` | search, by-id | map (search) or map (by-id) |
| `slo` | search, by-id history | map (search) or scalar `float64` (by-id) |

## Documentation

- [Installation](install.md) — install from a release binary URL; RBAC notes
- [Configuration](configuration.md) — full config schema, credential resolution, site values, result shapes
- [Migrating from the web provider](migration-web-provider.md) — migrating from the built-in `web` provider

## Examples

- [examples/metrics-analysistemplate.yaml](https://github.com/argoproj-labs/rollouts-plugin-metric-datadog/blob/main/examples/metrics-analysistemplate.yaml) — error-rate gate using v2 multi-query formula
- [examples/monitor-analysistemplate.yaml](https://github.com/argoproj-labs/rollouts-plugin-metric-datadog/blob/main/examples/monitor-analysistemplate.yaml) — monitor group search gate
- [examples/slo-analysistemplate.yaml](https://github.com/argoproj-labs/rollouts-plugin-metric-datadog/blob/main/examples/slo-analysistemplate.yaml) — SLO search gate
- [examples/multi-source-analysistemplate.yaml](https://github.com/argoproj-labs/rollouts-plugin-metric-datadog/blob/main/examples/multi-source-analysistemplate.yaml) — monitors + SLOs combined (OR-of-failures)

## Quick start

Add the plugin to the `argo-rollouts-config` ConfigMap and reference it in an AnalysisTemplate:

```yaml
# ConfigMap
metricProviderPlugins: |-
  - name: "argoproj-labs/rollouts-plugin-metric-datadog"
    location: "file://./plugin-bin/argoproj-labs/rollouts-plugin-metric-datadog"

# AnalysisTemplate metric entry
provider:
  plugin:
    argoproj-labs/rollouts-plugin-metric-datadog:
      tags: ["service:my-service", "env:production"]
      metrics:
        apiVersion: v2
        queries:
          a: "sum:trace.http.request.hits{*}.as_count()"
          b: "sum:trace.http.request.errors{*}.as_count()"
        formula: "(a - b) / a"
successCondition: "result >= 0.95"
```
