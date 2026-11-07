# Migrating from the `web` provider to this plugin

The Argo Rollouts built-in `web` metric provider can query Datadog. To do this, you construct the API URL manually, pass credentials as HTTP headers, and URI-encode tag filters into the query string. This plugin replaces that pattern with structured configuration, in-process credential resolution, and automatic tag merging. It keeps the same `failureCondition`/`successCondition` expressions.

---

## Before: `web` provider

```yaml
apiVersion: argoproj.io/v1alpha1
kind: AnalysisTemplate
metadata:
  name: datadog-error-rate-web
spec:
  args:
    - name: api-key
      valueFrom:
        secretKeyRef:
          name: datadog
          key: api-key
    - name: app-key
      valueFrom:
        secretKeyRef:
          name: datadog
          key: app-key
  metrics:
    - name: error-rate
      interval: 1m
      provider:
        web:
          url: >-
            https://api.datadoghq.com/api/v1/query?from={{- (now | unixEpoch | subtract 300) -}}&to={{- now | unixEpoch -}}&query=sum%3Atrace.http.request.errors%7Bservice%3Amy-cool-service%2Cenv%3Aproduction%7D.as_count()
          headers:
            - key: DD-API-KEY
              value: "{{args.api-key}}"
            - key: DD-APPLICATION-KEY
              value: "{{args.app-key}}"
          jsonPath: "{$.series[0].pointlist[-1][1]}"
      successCondition: "result >= 0.95"
```

Pain points:
- You must URI-encode tags into the URL by hand.
- The template passes credentials through `args` that reference a SecretKeyRef. The credentials appear in the AnalysisRun spec.
- Template functions compute the time window (`from`/`to`). This calculation is fragile.
- There is no rate limiting or caching across concurrent rollout waves.

---

## After: this plugin

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
              apiVersion: v1
              query: "sum:trace.http.request.errors{*}.as_count()"
      successCondition: "result >= 0.95"
```

What changed:
- The template has no credential args. The plugin reads credentials automatically from the `datadog` Secret in the controller namespace (resolution step 3). The existing `datadog` Secret (`api-key` / `app-key` keys) drops in unchanged. You need no new Secret and no new `args`.
- The tags are structured. `tags` is a YAML list, and the plugin merges the tags into the query's `{...}` scope brace. You do not need URI encoding.
- The plugin computes the time window in-process. It computes `[now-5m, now]` automatically from `metrics.interval` (default 5m). You do not need template arithmetic.
- Rate limiting and caching are on by default across all concurrent measurements.
- The `successCondition` stays the same. The scalar result shape is identical to what `jsonPath` extracts from the v1 API.

---

## Monitor migration

Before (web provider calling the search endpoint):

```yaml
provider:
  web:
    url: "https://api.datadoghq.com/api/v1/monitor/groups/search?query=service%3Amy-cool-service+env%3Aproduction+muted%3Afalse"
    headers:
      - key: DD-API-KEY
        value: "{{args.api-key}}"
      - key: DD-APPLICATION-KEY
        value: "{{args.app-key}}"
    jsonPath: "{$.counts.status}"
failureCondition: "any(result, {.name == 'Alert' && .count > 0})"
```

After:

```yaml
provider:
  plugin:
    argoproj-labs/rollouts-plugin-metric-datadog:
      tags: ["service:my-cool-service", "env:production"]
      monitor:
        query: "muted:false"
failureCondition: "any(result.counts.status, {.name == 'Alert' && .count > 0})"
```

The plugin returns the full `SearchMonitorGroupsResponse` as `result`. For this reason, the path becomes `result.counts.status`, not bare `result`. Update the condition to use this path.

---

## Credential reuse

The plugin's resolution step 3 reads a Secret with the literal name `datadog` from the controller namespace. It expects the `api-key` and `app-key` keys. This matches the standard Datadog Secret that the `web` provider previously referenced through `secretKeyRef`. You do not need to change the Secret. Remove the `args` block from the AnalysisTemplate. The plugin then resolves credentials automatically.

If your cluster uses a different Secret name, set `secretRef.name` explicitly:

```yaml
argoproj-labs/rollouts-plugin-metric-datadog:
  secretRef:
    name: my-datadog-secret
  tags: ["service:my-cool-service", "env:production"]
  metrics:
    query: "..."
```

See [configuration.md](configuration.md#credential-resolution-order) for the full resolution order and the secretRef escalation note.
