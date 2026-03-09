# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`flow` is a task orchestration CLI tool that runs workflows defined in YAML configuration files (similar to GitHub Actions). Built with Go and [cobra](https://github.com/spf13/cobra) for CLI framework.

## Build & Development Commands

```bash
make build      # Build binary to ./bin/flow
make test       # Run all tests (go test ./...)
make fmt        # Format code (go fmt ./...)
make vet        # Vet code (go vet ./...)
make tidy       # Tidy dependencies (go mod tidy)
make clean      # Remove build artifacts
make tools      # Install goreleaser
```

Single test: `go test -run TestName ./path/to/package`

## Release

- `make release type=patch|minor|major` — dry-run by default, add `dryrun=false` to execute
- `make re-release tag=vX.Y.Z dryrun=false` — recreate an existing release
- Tags pushed to origin trigger GitHub Actions → GoReleaser builds binaries for linux/darwin (amd64/arm64/arm)

## Architecture

- `main.go` — entrypoint, calls `cmd.Execute()`
- `cmd/` — cobra command definitions. Each command file registers itself via `init()` calling `rootCmd.AddCommand()`
- `internal/version/` — version info injected at build time via ldflags (`Version`, `CommitSHA`, `BuildTime`)
- `.product_name` — contains the binary name (`flow`), read by Makefile
- `.goreleaser.yaml` — GoReleaser v2 config with ldflags for version injection
