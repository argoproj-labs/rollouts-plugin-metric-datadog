# Troubleshooting

Every plugin error gives a measurement in the `Error` phase. The error text is in the `message` field of the measurement. To read it, run `kubectl get analysisrun <name> -n <namespace> -o yaml` and look under `status.metricResults[].measurements[]`.

In the tables, `<ns>`, `<name>`, and similar words stand for values from your configuration.

## Configuration errors

| Message | Cause | Fix |
|---|---|---|
| `no plugin config found under "argoproj-labs/rollouts-plugin-metric-datadog"` | The key under `provider.plugin` is not the plugin name. | Use the exact key `argoproj-labs/rollouts-plugin-metric-datadog`. |
| `invalid plugin config JSON: ...` | The plugin block has a value of the wrong type, for example a string for `timeoutSeconds`. | Compare the block with the [configuration reference](configuration.md). |
| `exactly one of metrics/monitor/slo must be set (found <n>)` | The metric has no source block, or more than one. | Keep one of `metrics`, `monitor`, or `slo`. To use several sources, see [Combine monitors and SLOs](../how-to/combine-sources.md). |
| `secretRef.namespaced=true requires a non-empty name` | `secretRef.namespaced` is `true`, and `secretRef.name` is empty. | Set `secretRef.name`. |
| `tags cannot be combined with a by-id mode (monitor.id/slo.id); use search mode for tag filtering` | The metric sets `tags` and `monitor.id` or `slo.id`. | Remove `tags`, or remove the `id` to use search mode. |
| `metrics.apiVersion must be v1 or v2, got "<value>"` | `metrics.apiVersion` has another value. | Set `v1` or `v2`, or remove the field to use `v2`. |
| `metrics requires exactly one of query or queries` | The `metrics` block has both `query` and `queries`, or neither. | Keep one of the two. |
| `metrics.formula requires queries` | `metrics.formula` is set together with `query`. | Move the query into `queries`, or remove `formula`. |
| `metrics with more than one query requires a formula` | `queries` has two or more entries, and `formula` is empty. | Add a `formula` that combines the named queries. |
| `metrics.aggregator is valid only for apiVersion v2` | `aggregator` is set with `apiVersion: v1`. | Remove `aggregator`, or use `v2`. |
| `invalid interval "<value>": ...` | `metrics.interval` or `slo.interval` is not a duration. | Use a Go duration such as `5m`, or days such as `7d`. |

## Credential and endpoint errors

| Message | Cause | Fix |
|---|---|---|
| `reading secret <ns>/<name>: secrets "<name>" not found` | The Secret does not exist in that namespace. | Create the Secret, or fix `secretRef.name` and `secretRef.namespaced`. Without a `secretRef`, the plugin reads the `datadog` Secret in the controller namespace. |
| `reading secret <ns>/<name>: ... forbidden ...` | The controller ServiceAccount cannot read Secrets in that namespace. | Grant the access. See [Install the plugin](../how-to/install.md#rbac). |
| `secret <ns>/<name> missing api-key/app-key` | The Secret has no `api-key` key or no `app-key` key, or one of them is empty. | Add both keys to the Secret. |
| `DD_API_KEY set but DD_APP_KEY missing` | The controller has `DD_API_KEY` in its environment but not `DD_APP_KEY`. | Set `DD_APP_KEY` on the controller, or remove `DD_API_KEY`. |
| `address "<value>" uses scheme "http"; only https is allowed ...` | `address`, the Secret `address` key, or `DD_ADDRESS` starts with `http://`. | Use an `https://` URL. |
| `address "<value>" must be a full URL with scheme and host` | The address has no scheme or no host, for example `api.datadoghq.com`. | Use a full URL such as `https://api.datadoghq.com`. |
| `invalid address "<value>": ...` | The address is not a valid URL. | Fix the URL. |

## Datadog API errors

Each Datadog error starts with the name of the call: `datadog v2 scalar query:`, `datadog v1 query:`, `datadog SearchMonitorGroups:`, `datadog GetMonitor:`, `datadog SearchSLO:`, or `datadog GetSLOHistory:`.

If `metadata.error_class` is `datadog`, Datadog itself failed, and the phase follows `onError`. A measurement can then be `Successful` or `Inconclusive` and still have an error in `message`. See [Datadog errors (onError)](configuration.md#datadog-errors-onerror).

| Message ends with | Cause | Fix |
|---|---|---|
| `401 Unauthorized` or `403 Forbidden` | Datadog rejected the API key or the application key. The keys can belong to another site, or the application key scopes do not allow the call. | Make sure that both keys are correct, that `site` matches your Datadog organization, and that the application key can read the source. |
| `404 Not Found` | The monitor or SLO ID does not exist in this organization. | Fix `monitor.id` or `slo.id`, and make sure that `site` is correct. |
| `429 Too Many Requests` | Datadog throttled the plugin. | See [Tune timeouts for rate limits](../how-to/tune-rate-limits.md). |
| `context deadline exceeded` | The call took longer than `timeoutSeconds`. | Raise `timeoutSeconds`, or shorten `metrics.interval`. See [Tune timeouts for rate limits](../how-to/tune-rate-limits.md). |
| `400 Bad Request` | Datadog rejected the query, for example a syntax error in a metric query or formula. | Run the query in the Datadog metrics explorer, then copy `metadata.resolvedQuery` from the measurement to compare. |

## Condition errors

| Message | Cause | Fix |
|---|---|---|
| `could not evaluate successCondition "...": metric result is nil or empty: no data returned from the metric provider` | A metric query or SLO by-id call returned no data. The same message appears for `failureCondition`. | Make sure that the query matches data in the window. To give no data a value, use `default(result, <value>)`. See [About the result model](../explanation/result-model.md#errors-are-never-failures). |
| `could not evaluate failureCondition "...": reflect: call of reflect.Value.Len on zero Value` | A condition calls `any`, `all`, or `none` on a list field that the result does not have. An example is a response without `counts.status`. | Guard the field with `!= nil`, as in `result.counts.status != nil && any(...)`. |
| `could not evaluate ...: cannot fetch <field> from <nil>` | The condition reads a field below a field that the result does not have. | Fix the field path, or guard the parent field with `!= nil`. |

## Other symptoms

| Symptom | Cause | Fix |
|---|---|---|
| The measurement phase is `Inconclusive`. | Neither `successCondition` nor `failureCondition` was true. | Make sure that the two conditions cover all results, or set only one of them. |
| The controller log shows `init kube client: in-cluster config: ...`. | The plugin process cannot reach the Kubernetes API. It runs only inside the controller pod. | Run the controller in the cluster with its ServiceAccount token mounted. |
| A monitor or SLO metric passes, but it watches the wrong resources. | The search uses only broad tags such as `env:production`. | Add a `service:` tag. See [About rate limiting and caching](../explanation/rate-limiting.md#why-search-metrics-need-a-service-tag). |
