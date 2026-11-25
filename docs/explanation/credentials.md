# About credentials and the secretRef risk

The plugin reads Datadog keys from Kubernetes Secrets or environment variables inside the controller process. This page explains why it works that way, and what risk comes with it.

## Why the plugin reads the keys itself

With the built-in `web` provider, an AnalysisTemplate passes the API key and the application key through `args` into HTTP headers. The keys then appear in the AnalysisRun objects. This plugin reads the keys itself, so they never enter a Rollout, AnalysisTemplate, or AnalysisRun.

## Why the resolution order looks like this

The plugin tries three places in order and stops at the first one that applies:

1. A `secretRef` on the metric.
2. The `DD_API_KEY` and `DD_APP_KEY` environment variables of the controller.
3. A Secret with the literal name `datadog` in the controller namespace.

The order matches the built-in Datadog provider of Argo Rollouts. As a result, a cluster that already has a `datadog` Secret with `api-key` and `app-key` keys works with no new configuration.

Each step either succeeds or ends the measurement with an error. If a metric names a `secretRef` and the plugin cannot read that Secret, the plugin does not try the environment variables. If `DD_API_KEY` is set without `DD_APP_KEY`, the measurement ends in the `Error` phase, and the plugin does not read the `datadog` Secret.

The plugin picks the endpoint in this order. The `address` field of the plugin configuration wins first. Next comes the address that belongs to the keys: the `address` key of the Secret, or `DD_ADDRESS`. If none of them is set, the plugin uses `site`. The plugin accepts only `https://` addresses, so the keys never travel in clear text.

## The secretRef risk

The plugin runs inside the controller and reads Secrets through the controller ServiceAccount. The standard Argo Rollouts install lets that ServiceAccount read Secrets in every namespace.

The `secretRef.name` value is unrestricted. With `namespaced: true`, anyone who can write an AnalysisTemplate or AnalysisRun in a namespace can make the controller read any Secret in that namespace. If that Secret has `api-key` and `app-key` keys, the plugin sends their values to Datadog. For this reason, treat "can write AnalysisTemplates in namespace N" as "can read the Datadog keys in all Secrets in N".

The `address` field widens this risk. A template author can set `address` to any HTTPS host, and the plugin then sends the resolved keys to that host. This also applies to the keys from the default `datadog` Secret in the controller namespace.

To limit the risk, use an admission policy, for example OPA or Kyverno. The policy can reject unknown `secretRef.name` values and any `address` value that is not your Datadog endpoint. For the RBAC side, see [Install the plugin](../how-to/install.md#rbac).
