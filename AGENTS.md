# AGENTS.md

This file gives rules and instructions to AI coding agents, for example Claude Code, Cursor, and Copilot.
`CLAUDE.md` is a symlink to this file, for tools that read that name.

These rules prefer caution to speed. For trivial tasks, use your judgment.

---

## 1. Think before coding

Do not assume. Do not hide confusion. Say what the tradeoffs are.

Before you implement:
- State your assumptions explicitly. If you are not sure, ask.
- If multiple interpretations exist, present them. Do not pick one silently.
- If a simpler approach exists, say so. When pushback is warranted, push back.
- If something is unclear, stop. Name what confuses you. Ask.

## 2. Simplicity first

Write the minimum code that solves the problem. Write nothing speculative.

- Add no features beyond what the user asked for.
- Add no abstractions for single-use code.
- Add no "flexibility" or "configurability" that nobody requested.
- Add no error handling for impossible scenarios.
- If you write 200 lines and 50 lines are enough, rewrite it.

Ask yourself: "Will a senior engineer say this is overcomplicated?" If the answer is yes, simplify.

## 3. Surgical changes

Touch only what you must. Clean up only your own mess.

When you edit existing code:
- Do not "improve" adjacent code, comments, or formatting.
- Do not refactor things that are not broken.
- Even if you prefer a different style, match the existing style.
- If you see unrelated dead code, mention it. Do not delete it.

When your changes create orphans:
- Remove imports, variables, and functions that YOUR changes made unused.
- If nobody asked you to, do not remove pre-existing dead code.

Use this test: every changed line must trace directly to the user's request.

## 4. Goal-driven execution

Define success criteria. Loop until the criteria pass.

Turn each task into a verifiable goal:
- "Add validation" → "Write tests for invalid inputs, then make them pass"
- "Fix the bug" → "Write a test that reproduces it, then make it pass"
- "Refactor X" → "Make sure that tests pass before and after"

For multi-step tasks, state a brief plan:
```
1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]
```

Strong success criteria let you loop independently. Weak criteria, for example "make it work", need constant clarification.

---

## Project-specific rules

### Commands

```bash
make build      # CGO_ENABLED=0 build for host platform → dist/rollouts-plugin-metric-datadog
make release    # cross-compile: linux/darwin × amd64/arm64 → dist/
make test       # go test ./... -race -count=1
make vet        # go vet ./...
test -z "$(gofmt -l .)"   # formatting gate (also enforced by CI)

# single package / single test
go test ./internal/datasource -run TestMetrics -race -count=1
```

### Every commit needs a DCO sign-off

The DCO check blocks any PR that contains a commit without a `Signed-off-by:` trailer that matches its author. Commit with `git commit -s`. As an alternative, run `git config core.hooksPath .githooks` once. Then `.githooks/prepare-commit-msg` adds the trailer for you.

### Never run `go mod tidy` casually

`go mod tidy` strips `github.com/DataDog/zstd`, a cgo-only transitive dependency. This breaks the `-race` build. If you must tidy, restore the dependency immediately:

```bash
go get github.com/DataDog/zstd@v1.5.2
```

Dependabot ignores this dependency for the same reason. See `.github/dependabot.yml`.

### Errors must be `MarkMeasurementError`, never `Failed`

Any error in `Run()` must produce an `Error` measurement phase, not `Failed`. `failureLimit` defaults to 0. If you map a Datadog throttle to `Failed`, the rollout aborts on infrastructure flakiness instead of a real regression.

### Test guard: never remove the `DD_*` env check

When `DD_API_KEY` or `DD_APP_KEY` is set in the environment, `Run`-level tests skip or fail fast. When you add cases, keep that guard.

### Testing conventions

- Table-test data sources against `net/http/httptest` stubs. Assert the request shape and the response parsing. Do not assert against live Datadog.
- `RpcPlugin` exposes the `selectSource` and `newClient` indirections. Use them to inject fakes without a real kube client or a real Datadog endpoint.

---

## Reference docs

- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md): the full design spec (architecture, result model, rate-limiting, phasing)
- [docs/configuration.md](docs/configuration.md): the configuration schema, credential resolution, and site values
- [docs/install.md](docs/install.md): installation from a release binary
- [CONTRIBUTING.md](CONTRIBUTING.md): how to add a data source, run tests, and submit a PR

---

You can see that these rules work from these results:
- Diffs have fewer unnecessary changes.
- Fewer rewrites come from overcomplication.
- Clarifying questions come before implementation, not after mistakes.
