# Agent Development Guide

A file for [guiding coding agents](https://agents.md/).

## Commands

- **Test:** `make test`. Prefer a package: `go test ./pkg/<pkg>/... -race`;
  focus with `FOCUS="<spec text>"`.
- **Lint/codegen:** `make verify`. Run after changing `pkg/apis`.

## Code

- Inject a `clock.Clock`; never call `time.Now()` directly.
- Use `serrors`, wrap with `%w`, and return errors to the reconciler rather
  than logging and dropping them.
- In tests, use the `test.*` builders and `Expect*` helpers from
  `sigs.k8s.io/karpenter/pkg/test` (and this repo's `pkg/test`), and assert on
  observable behavior, not internal calls. Reuse helpers whenever possible.
