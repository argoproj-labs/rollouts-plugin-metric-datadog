# About the result model

The plugin never decides by itself whether a rollout passes. Each measurement returns a value, and the conditions in the AnalysisTemplate decide the phase. This page explains how that works and why.

## Conditions decide the phase

The plugin calls `evaluate.EvaluateResult` from Argo Rollouts, which every metric provider uses. As a result, `successCondition` and `failureCondition` work the same way as with Prometheus, the `web` provider, or the built-in Datadog provider.

The evaluator gives one of three phases:

- If `failureCondition` is true, the phase is `Failed`.
- If only `successCondition` is true, the phase is `Successful`.
- If neither condition is true, the phase is `Inconclusive`.

If a metric has only a `successCondition`, a false result counts as `Failed`. If a metric has only a `failureCondition`, a false result counts as `Successful`.

## Structured results keep web-provider field names

The metrics source and SLO by-id mode return one number. The monitor source and SLO search mode return the Datadog response as a structure.

The Datadog Go SDK returns typed Go structs. The plugin converts each struct to JSON and back into a plain map before it evaluates the conditions. The map then has the same field names as the raw Datadog API JSON. As a result, a condition such as `result.counts.status`, written for the `web` provider, keeps working after a migration. A test in the repository feeds canned Datadog JSON through this conversion and evaluates the `web`-provider conditions against it.

The monitor search result drops one field, `counts.tag`. Datadog computes that facet over every matched monitor group and keeps up to 1,000 entries. Its size grows with the organization, not with the query. In one real organization it was 54 KB of an 80 KB measurement value. Argo Rollouts keeps 10 measurements per metric, so this field cost about 800 KB of AnalysisRun status per metric. That can push a template with several metrics past the 1.5 MB request limit of etcd.

## Errors are never failures

The plugin turns every error into an `Error` measurement. This covers a bad configuration, a missing Secret, a Datadog error or timeout, and a condition that cannot evaluate the result. For the reason, see [About rate limiting and caching](rate-limiting.md#why-throttling-gives-error-not-failed).

Empty results follow the source type. A metrics query or SLO by-id call with no data returns `nil`. Most conditions cannot compare `nil`, so the measurement becomes an `Error` with the message `metric result is nil or empty`. Even when a monitor or SLO search matches nothing, it returns the real response. The condition decides what an empty match means, for example `result.counts.status == nil`.

## One source per metric

Each metric entry has exactly one source block. To combine sources, an AnalysisTemplate lists several metrics. If any metric fails, the AnalysisRun fails, so several metrics give an OR of failures. For example, one template can fail on a monitor alert or an SLO breach.

A single condition cannot combine two sources with AND. For example, no condition can say that a monitor alerts and the SLO budget is under 10%. The design leaves this out on purpose. See the non-goals in the [design spec](../ARCHITECTURE.md#non-goals).
