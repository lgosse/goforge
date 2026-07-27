# linters

[![Go Reference](https://pkg.go.dev/badge/github.com/lgosse/goforge/linters/plugin.svg)](https://pkg.go.dev/github.com/lgosse/goforge/linters/plugin)

The `linters` module is the planned home of GoForge lint rules packaged for
`golangci-lint`.

Its goal is to centralize project conventions that are specific enough not to
belong in general-purpose linters while keeping findings compatible with the
normal Go tooling workflow.

## Planned direction

- Correctness checks for GoForge contracts and common service patterns.
- Focused diagnostics with actionable messages.
- Compatibility with custom `golangci-lint` builds.
- Tests that demonstrate both accepted and rejected code.
- Versioned adoption independent of runtime GoForge modules.

## Status

The plugin package currently contains scaffolding only. No public analyzer or
stable plugin registration is available yet.

## Development

Run module checks from this directory:

```sh
go test ./...
go vet ./...
```

## License

This module is available under the [MIT License](./LICENCE.txt).
