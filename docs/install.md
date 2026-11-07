# Installing the Datadog metric plugin

When the controller starts, Argo Rollouts downloads the plugin binary from a GitHub release. Add the `sha256` checksum. The controller then verifies the binary before it runs it.

## Configure the plugin in the ConfigMap

Update (or create) the `argo-rollouts-config` ConfigMap in the same namespace as the controller:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: argo-rollouts-config
data:
  metricProviderPlugins: |-
    - name: "argoproj-labs/rollouts-plugin-metric-datadog"
      location: "https://github.com/argoproj-labs/rollouts-plugin-metric-datadog/releases/download/v0.1.0/rollouts-plugin-metric-datadog-linux-amd64"
      sha256: "<sha256 of the binary>"
```

If your controller nodes are arm64, use the `linux-arm64` binary. To get the checksum, download the binary from the [releases page](https://github.com/argoproj-labs/rollouts-plugin-metric-datadog/releases). Then run this command:

```bash
sha256sum rollouts-plugin-metric-datadog-linux-amd64
```

After you apply the ConfigMap, restart the controller.

Before you run an AnalysisRun, make sure that the controller log shows the handshake:

```
plugin: plugin started: path=/home/argo-rollouts/plugin-bin/argoproj-labs/rollouts-plugin-metric-datadog pid=15
plugin: using plugin: version=1
```

When you upgrade, update the version tag and the `sha256`. The [releases page](https://github.com/argoproj-labs/rollouts-plugin-metric-datadog/releases) lists all available versions.

The controller downloads the plugin again each time its pod starts. If GitHub is unreachable at that moment, the controller does not start. See [Some words of caution](https://argo-rollouts.readthedocs.io/en/stable/analysis/plugins/#some-words-of-caution) in the Argo Rollouts documentation.

---

## RBAC

### Default install (controller-namespace secret)

The default credential resolution (step 3 in [configuration.md](configuration.md#credential-resolution-order)) reads a Secret named `datadog` from the controller's own namespace. The standard `argo-rollouts` ClusterRole already grants `secrets/get` within its namespace. As a result, the default install requires no additional RBAC.

### Cross-namespace secretRef (`namespaced: true`)

If a metric uses `secretRef.namespaced: true`, the plugin reads the named Secret from the AnalysisRun's namespace through the controller ServiceAccount. You must grant the controller `get` on Secrets in those namespaces.

CAUTION: Operators must treat "can author AnalysisTemplates in namespace N" as equivalent to "can read all Secrets in N". To restrict this, enforce eligible Secret names by naming convention, label selector, or an admission policy (for example, OPA/Kyverno) that rejects unknown `secretRef.name` values. This is the secretRef escalation risk. The `secretRef.name` value is unrestricted. With `namespaced: true`, the plugin reads the named Secret from the AnalysisRun's namespace through the controller ServiceAccount. As a result, anyone who can author AnalysisTemplates or AnalysisRuns in a namespace can instruct the controller to read any Secret in that namespace. The controller then forwards the `api-key`/`app-key` of that Secret to Datadog.

Example RBAC for cross-namespace reads:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: argo-rollouts-secret-reader
  namespace: my-app-namespace
rules:
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: argo-rollouts-secret-reader
  namespace: my-app-namespace
subjects:
  - kind: ServiceAccount
    name: argo-rollouts
    namespace: argo-rollouts
roleRef:
  kind: Role
  apiGroup: rbac.authorization.k8s.io
  name: argo-rollouts-secret-reader
```
