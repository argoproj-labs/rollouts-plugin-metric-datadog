## What and why

<!-- Tell reviewers what this change does and why. Link the issue, for example "Fixes #12". -->

## How you tested it

<!-- List the tests that you added or ran. For a data source change, name the httptest cases. -->

## Checklist

- [ ] `make test`, `make vet`, `golangci-lint run`, and `test -z "$(gofmt -l .)"` pass.
- [ ] Every commit has a `Signed-off-by:` trailer (`git commit -s`).
- [ ] Errors in `Run()` produce an `Error` measurement phase, not `Failed`.
- [ ] If I ran `go mod tidy`, I restored `github.com/DataDog/zstd` with `go get github.com/DataDog/zstd@v1.5.2`.
- [ ] Tests that I added keep the `DD_API_KEY` and `DD_APP_KEY` environment guard.
- [ ] If I changed the configuration schema or behavior, I updated `docs/configuration.md` and `examples/`.
