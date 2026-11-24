# About rate limiting and caching

Datadog limits API calls per organization. Every rollout in a cluster draws from the same budget, so the plugin limits its own calls and shares results between measurements. This page explains how those parts work in Phase 1.

## The limiter

All measurements in one plugin process share one limiter. The limiter keeps a separate token bucket (a counter that refills at a fixed rate) for each Datadog rate-limit group.

The first call to an endpoint uses a default bucket of 10 requests per second. Datadog names the group of each endpoint in the `X-RateLimit-Name` response header. It also sends the limit in `X-RateLimit-Limit` and the period in `X-RateLimit-Period`. After the first response, the limiter moves that endpoint to a bucket for its group. It sets the bucket to half of the limit that Datadog reports, so other Datadog clients in the organization keep the other half.

The limiter also allows at most 16 requests in flight at the same time. If many canaries start together, this cap stops a burst.

In Phase 1, these values are fixed in the code. The `rateLimit` fields in the configuration have no effect.

## The cache

The cache does two jobs. First, if several measurements send the same query at the same time, the plugin makes one call. All of them get its result. Second, the plugin keeps each successful result for 30 seconds and serves it to later measurements that send the same query.

If two measurements have the same source block, `tags`, `site`, `address`, and Secret, they send the same query. The cache key also includes a time bucket, so results roll forward as the query window moves. The cache stores only successful results. If the call fails, the measurement gets the error, and the plugin never serves an older value instead.

If you set `cache.enabled: false` on a metric, the plugin skips the 30-second stored result and always calls Datadog. Concurrent identical queries still share one call, because that sharing costs no freshness.

## Why throttling gives Error, not Failed

A throttled call, a timeout, or a Datadog server error always gives an `Error` measurement. The AnalysisRun counts errors against `consecutiveErrorLimit`, which defaults to 4, and resets the count after a success. A `Failed` measurement counts against `failureLimit`, which defaults to 0, so one `Failed` aborts the rollout. If throttling gave `Failed`, a busy Datadog organization can abort a healthy rollout.

## Why search metrics need a service tag

A monitor or SLO search that uses only broad tags, for example `env:production`, matches resources of every service in that environment. Two rollouts of different services then send the same query, get the same result, and gate on each other's monitors.

A `service:` tag makes each query match one service. If a monitor or SLO search has neither a `service:` tag nor a `query`, the plugin logs a warning.

To size timeouts and limits for your cluster, see [Tune timeouts for rate limits](../how-to/tune-rate-limits.md).
