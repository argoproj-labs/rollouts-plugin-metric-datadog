# Contributing

## Prerequisites

- Go (version from `go.mod`)
- [`golangci-lint`](https://golangci-lint.run/welcome/install/) v2+

## Building

```bash
make build      # host platform, outputs dist/rollouts-plugin-metric-datadog
make release    # linux/darwin × amd64/arm64, outputs to dist/
```

## Testing

```bash
make test       # go test ./... -race -count=1
```

Tests are run with the race detector. `Run`-level tests guard against ambient `DD_API_KEY` / `DD_APP_KEY` environment variables — unset them before running locally if present.

To run a single package or test:

```bash
go test ./internal/datasource -run TestMetrics -race -count=1
```

## Linting

```bash
golangci-lint run   # runs errcheck, staticcheck, govet and others
make vet            # go vet only
```

## Adding a data source

Each source lives in its own file under `internal/datasource/` and implements two methods: `Query(ctx, client, cfg) (Result, error)` and `Key(cfg) string`. To add one:

1. Create `internal/datasource/<name>.go` implementing the interface.
2. Add a case for it in `internal/datasource/select.go`'s `Select()` function.
3. Add config fields in `internal/config/config.go` and validation in `ParseConfig`.

The core plugin (`internal/plugin/plugin.go`) does not change — rate-limiting, caching, and credential resolution are all handled there automatically.

## Sign your commits (DCO)

Every commit must carry a `Signed-off-by:` trailer matching its author, or the DCO check blocks the pull request. `git commit -s` adds it.

To never think about it again, enable the repo's hook once per clone:

```bash
git config core.hooksPath .githooks
```

`.githooks/prepare-commit-msg` then appends the trailer to any commit that lacks one, including `--amend` and rebases.

## Submitting changes

1. Fork the repo and create a branch from `main`.
2. Make your changes and ensure `make test`, `make vet`, and `golangci-lint run` all pass.
3. Sign off every commit (see above).
4. Open a pull request — CI runs the same checks automatically.
