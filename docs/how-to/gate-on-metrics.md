# Gate a rollout on a metric query

This guide sets up a canary step that compares a Datadog metric query for your service with a threshold. The example gates on the success rate of HTTP requests.

Before you start, install the plugin. See [Install the plugin](install.md).

## Create the AnalysisTemplate

1. Save this AnalysisTemplate as `datadog-error-rate.yaml`. Replace the tag values with the tags of your service.

    ```yaml
    apiVersion: argoproj.io/v1alpha1
    kind: AnalysisTemplate
    metadata:
      name: datadog-error-rate
    spec:
      metrics:
        - name: error-rate
          interval: 1m
          provider:
            plugin:
              argoproj-labs/rollouts-plugin-metric-datadog:
                tags: ["service:my-cool-service", "env:production"]
                metrics:
                  apiVersion: v2
                  queries:
                    a: "sum:trace.http.request.hits{*}.as_count()"
                    b: "sum:trace.http.request.errors{*}.as_count()"
                  formula: "(a - b) / a"
          successCondition: "result >= 0.95"
    ```

2. Apply the template in the namespace of your Rollout:

    ```bash
    kubectl apply -n my-namespace -f datadog-error-rate.yaml
    ```

The plugin merges `tags` into the `{*}` scope of each query. It sends `sum:trace.http.request.hits{service:my-cool-service,env:production}.as_count()` for query `a`. The formula gives one number, and `successCondition` compares it.

By default, the plugin queries the last 5 minutes. To change the window, set `metrics.interval`.

## Choose how the window becomes one number

The `aggregator` field turns the points in the window into one number. The default, `last`, uses the last point. To use the whole window, set `aggregator: sum`:

```yaml
metrics:
  apiVersion: v2
  aggregator: sum
  queries:
    a: "sum:trace.http.request.hits{*}.as_count()"
    b: "sum:trace.http.request.errors{*}.as_count()"
  formula: "(a - b) / a"
```

## Decide what no data means

If the query returns no data, the result is `nil`, and `result >= 0.95` cannot evaluate. The measurement then ends in the `Error` phase. To let a new service with no traffic pass, give `nil` a value with the `default` function:

```yaml
successCondition: "default(result, 1.0) >= 0.95"
```

A test such as `result == nil || ...` does not work, because Argo Rollouts compiles the condition against the type of `result`.

## Add the analysis to the Rollout

Add an analysis step after the first traffic step of the canary:

```yaml
spec:
  strategy:
    canary:
      steps:
        - setWeight: 20
        - analysis:
            templates:
              - templateName: datadog-error-rate
```

To make sure that the plugin sends the query that you expect, read `metadata.resolvedQuery` in the AnalysisRun status. For all fields, see the [configuration reference](../reference/configuration.md#metrics-source).
