# Gate a rollout on Datadog monitors

This guide sets up a canary step that fails on an alerting Datadog monitor for your service.

Before you start, install the plugin. See [Install the plugin](install.md). This guide uses monitors that carry a `service:` tag and an `env:` tag.

## Create the AnalysisTemplate

1. Save this AnalysisTemplate as `datadog-monitors.yaml`. Replace the tag values with the tags of your service.

    ```yaml
    apiVersion: argoproj.io/v1alpha1
    kind: AnalysisTemplate
    metadata:
      name: datadog-monitors
    spec:
      metrics:
        - name: monitors
          interval: 1m
          failureLimit: 2
          provider:
            plugin:
              argoproj-labs/rollouts-plugin-metric-datadog:
                tags: ["service:my-cool-service", "env:production"]
                monitor:
                  query: "muted:false"
          failureCondition: "result.counts.status != nil && any(result.counts.status, {.name == 'Alert' && .count > 0})"
          successCondition: "result.counts.status == nil || any(result.counts.status, {.name != 'Alert'})"
    ```

2. Apply the template in the namespace of your Rollout:

    ```bash
    kubectl apply -n my-namespace -f datadog-monitors.yaml
    ```

The plugin joins `tags` and `query` into one monitor search. The `muted:false` term leaves out muted monitors. If any monitor group in the result is in `Alert`, the measurement is `Failed`. If the search matches no monitor groups, `counts.status` is an empty list. Neither condition is then true, so the measurement is `Inconclusive`, and Argo Rollouts pauses the rollout.

To let a search with no groups pass, use this `successCondition` instead:

```yaml
successCondition: "result.counts.status == nil || none(result.counts.status, {.name == 'Alert' && .count > 0})"
```

An empty search can also mean that the tags match nothing. Before you use this condition, make sure that `metadata.resolvedQuery` finds your monitors.

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
              - templateName: datadog-monitors
```

To check the monitors during the whole rollout, put the template under `spec.strategy.canary.analysis` instead.

## Make sure that the gate works

1. Start a rollout, for example with a new image tag.
2. List the AnalysisRuns:

    ```bash
    kubectl get analysisrun -n my-namespace
    ```

3. Show the AnalysisRun of the rollout:

    ```bash
    kubectl get analysisrun <name> -n my-namespace -o yaml
    ```

4. Under `status.metricResults`, find the `monitors` metric. Make sure that `metadata.resolvedQuery` of each measurement matches the monitors that you expect.

## Gate on one monitor

To watch one monitor, use its ID. Remove `tags`, because the plugin rejects tags in by-id mode.

```yaml
provider:
  plugin:
    argoproj-labs/rollouts-plugin-metric-datadog:
      monitor:
        id: 12345678
failureCondition: "result.overall_state == 'Alert'"
```

The result is the full monitor object. For all fields and result shapes, see the [configuration reference](../reference/configuration.md#monitor-source).
