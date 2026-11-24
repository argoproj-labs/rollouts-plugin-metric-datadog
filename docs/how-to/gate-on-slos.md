# Gate a rollout on SLOs

This guide sets up a canary step that fails on a breached Datadog SLO for your service.

Before you start, install the plugin. See [Install the plugin](install.md). This guide uses SLOs that carry a `service:` tag and an `env:` tag.

## Create the AnalysisTemplate

1. Save this AnalysisTemplate as `datadog-slos.yaml`. Replace the tag values with the tags of your service.

    ```yaml
    apiVersion: argoproj.io/v1alpha1
    kind: AnalysisTemplate
    metadata:
      name: datadog-slos
    spec:
      metrics:
        - name: slos
          interval: 1m
          failureLimit: 2
          provider:
            plugin:
              argoproj-labs/rollouts-plugin-metric-datadog:
                tags: ["service:my-cool-service", "env:production"]
                slo: {}
          failureCondition: "any(result.slos, {any(.overall_status, {.state == 'breached'})})"
          successCondition: "all(result.slos, {all(.overall_status, {.state != 'breached'})})"
    ```

2. Apply the template in the namespace of your Rollout:

    ```bash
    kubectl apply -n my-namespace -f datadog-slos.yaml
    ```

The plugin searches for SLOs with the tags in `tags`. Each SLO in the result has an `overall_status` list with one entry per time window. The `state` of an entry is `breached`, `warning`, `ok`, or `no_data`.

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
              - templateName: datadog-slos
```

## Fail on a low error budget

To fail before an SLO breaches, compare the remaining error budget. The `error_budget_remaining` field is a percentage, and it can be empty. Test for an empty value first:

```yaml
failureCondition: "any(result.slos, {any(.overall_status, {.error_budget_remaining != nil && .error_budget_remaining < 10})})"
```

## Gate on one SLO

To watch one SLO, use its ID. Remove `tags`, because the plugin rejects tags in by-id mode. The result is the SLI value over `interval`, as a number.

```yaml
provider:
  plugin:
    argoproj-labs/rollouts-plugin-metric-datadog:
      slo:
        id: "abc123def456"
        interval: "7d"
successCondition: "result >= 99.9"
```

By-id mode does not return the error budget. To use the error budget, use search mode.

For all fields and result shapes, see the [configuration reference](../reference/configuration.md#slo-source).
