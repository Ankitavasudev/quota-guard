# Contributing to quota-guard

Thanks for your interest. quota-guard is a small, focused Kubernetes operator and
contributions are welcome.

## Before you start

- Read the [README](README.md) for the architecture and CRD shape.
- Open an issue first for anything larger than a small fix, so we can agree on
  the approach.

## Development setup

```bash
make test   # unit tests (fake client, no cluster needed)
make vet
make build
```

Tests use `controller-runtime/pkg/client/fake`. New behaviour should come with a
test in `internal/controller/*_test.go`.

## Pull requests

- Keep PRs small and focused.
- Run `make test && make vet` before pushing.
- Update the README if you change the CRD, metrics, or CLI flags.
- Add a changeset-style note in the PR description.

## Code style

- Go code must be `gofmt`/`goimports` clean.
- Follow the existing reconcile-loop pattern: predicate filter on Pod events,
  write status via `meta.SetStatusCondition`, emit events on transitions.

## License

By contributing you agree your contributions are licensed under the MIT License.