# Tune timeouts for rate limits

If Datadog throttles the plugin and measurements end in the `Error` phase, use this guide. The measurement message then contains `429 Too Many Requests` or `context deadline exceeded`.

## Give the retry time to run

The SDK retries a 429 response after the wait in the `X-RateLimit-Reset` header. It stops retrying after four fifths of `timeoutSeconds`. For `/api/v1/query`, the wait can be about 60 seconds, so the default timeout of 30 seconds ends before the retry runs.

1. Set `timeoutSeconds` to 90 in the plugin configuration. The retry window is then 72 seconds, which covers a 60-second wait.
2. If one retry is not enough, raise `retry.maxRetries` above the default of 3.

```yaml
provider:
  plugin:
    argoproj-labs/rollouts-plugin-metric-datadog:
      timeoutSeconds: 90
      retry:
        maxRetries: 5
```

## Let rollouts share results

The plugin serves one result to all metrics that send the same query within 30 seconds. If two metrics have the same source block, `tags`, `site`, `address`, and Secret, they send the same query.

1. Keep `cache.enabled` at its default of `true`.
2. For metrics that watch the same service, use the same `tags` list in each AnalysisTemplate.
3. Set the metric `interval` to `1m` or longer.

## Allow for short throttling

A throttled measurement is an `Error`, not a `Failed`. The controller aborts the rollout after `consecutiveErrorLimit` errors in a row. The default is 4.

1. If throttling lasts longer than four measurements, set `consecutiveErrorLimit` on the metric to a higher value.
2. If many rollouts start at the same time, stagger the deploys.

```yaml
metrics:
  - name: monitors
    interval: 1m
    consecutiveErrorLimit: 8
```

## Ask Datadog for a higher limit

If the steps above are not enough, ask Datadog support to raise the rate limit for your organization. See [Datadog API rate limits](https://docs.datadoghq.com/api/latest/rate-limits/).

For how the limiter and the cache work, see [About rate limiting and caching](../explanation/rate-limiting.md).
