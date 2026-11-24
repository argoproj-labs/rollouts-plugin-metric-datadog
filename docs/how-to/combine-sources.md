# Combine monitors and SLOs

This guide sets up one analysis that fails on an alerting monitor or on a breached SLO.

Before you start, install the plugin. See [Install the plugin](../install.md).

## Create the AnalysisTemplate

Each metric entry has exactly one source block. To combine sources, list one metric for each source in the same AnalysisTemplate.

1. Save this AnalysisTemplate as `datadog-monitors-and-slos.yaml`. Replace the tag values with the tags of your service.

    ```yaml
    apiVersion: argoproj.io/v1alpha1
    kind: AnalysisTemplate
    metadata:
      name: datadog-monitors-and-slos
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
          failureCondition: "result.counts.status != nil && any(result.counts.status, {.name == 'Alert' && .count > 0})"
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

2. Apply the template in the namespace of your Rollout:

    ```bash
    kubectl apply -n my-namespace -f datadog-monitors-and-slos.yaml
    ```

3. Add the template to an analysis step of the canary, as in [Gate a rollout on Datadog monitors](gate-on-monitors.md#add-the-analysis-to-the-rollout).

## How the metrics combine

Argo Rollouts measures each metric on its own schedule. If any metric fails, the AnalysisRun fails. The template above therefore fails on a monitor alert or an SLO breach.

Each metric keeps its own `interval`, `failureLimit`, and `consecutiveErrorLimit`. For example, to check SLOs less often than monitors, set `interval: 5m` on the `slos` metric only.

To add a metric query to the same analysis, add a third metric with a `metrics` block. See [Gate a rollout on a metric query](gate-on-metrics.md).

A single condition cannot combine two sources with AND. For the reason, see [About the result model](../explanation/result-model.md#one-source-per-metric).
