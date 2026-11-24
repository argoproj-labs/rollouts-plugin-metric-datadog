# Result shapes and measurement metadata

This page lists the `result` value of each source. It also lists the fields that the plugin writes into the AnalysisRun status.

## Result shapes

| Source | Mode | `result` type | Example condition |
|---|---|---|---|
| `metrics` | v1 or v2 | `float64`. `nil` for no data. | `result >= 0.95` |
| `monitor` | search | map with `result.counts.status[].{name, count}` | `result.counts.status != nil && any(result.counts.status, {.name == 'Alert' && .count > 0})` |
| `monitor` | by-id | map with the full Monitor JSON | `result.overall_state == 'Alert'` |
| `slo` | search | map with `result.slos[].overall_status[].{state, error_budget_remaining, ...}` | `any(result.slos, {any(.overall_status, {.state == 'breached'})})` |
| `slo` | by-id | `float64` SLI value. `nil` for no data. | `result >= 99.9` |

Map results use the field names of the Datadog API JSON. A field that the Datadog response leaves out is missing from the map, and an empty list stays an empty list. The plugin drops `counts.tag` from monitor search results.

Values of the monitor `overall_state` field: `Alert`, `Warn`, `No Data`, `OK`, `Ignored`, `Skipped`, `Unknown`.

Values of the SLO `state` field: `breached`, `warning`, `ok`, `no_data`.

## Measurement fields

The plugin fills these fields of each entry in `status.metricResults[].measurements[]`:

| Field | Content |
|---|---|
| `phase` | `Successful`, `Failed`, or `Inconclusive` from the conditions. `Error` for any plugin or Datadog error. |
| `value` | The result as text. A number is printed as a decimal. A map is printed as JSON. `nil` is printed as an empty string. |
| `message` | The error text of an `Error` measurement. See [Troubleshooting](troubleshooting.md). |
| `metadata` | The keys in the next table. The plugin writes no metadata on an `Error` measurement. |

## Measurement metadata keys

| Key | Sources | Value |
|---|---|---|
| `source` | All | `metrics`, `monitor`, or `slo`. |
| `cache` | All | `fresh` for a new call, `coalesced` for a call shared with concurrent measurements, `cached` for a stored result. |
| `apiVersion` | `metrics` | `v1` or `v2`. |
| `mode` | `monitor`, `slo` | `search` or `id`. |
| `resolvedQuery` | `metrics`, and search mode of `monitor` and `slo` | The query that the plugin sent, after it merged `tags`. For v2 metrics, a map from query name to query, for example `map[a:sum:trace.http.request.hits{service:my-cool-service}.as_count()]`. |

## Metric metadata keys

The plugin also fills `status.metricResults[].metadata` once for each metric:

| Key | Value |
|---|---|
| `source` | `metrics`, `monitor`, or `slo`. |
| `tags` | The `tags` list, for example `[service:my-cool-service env:production]`. For an empty `tags` list, the key is missing. |
