# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`flow` is a task orchestration CLI tool that runs workflows defined in YAML configuration files (similar to GitHub Actions). Built with Go and [cobra](https://github.com/spf13/cobra) for CLI framework.

## Environment

Go toolchain is managed via direnv (automatically loaded in the shell).

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
  - `run.go` — `flow run <workflow>` command: config読み込み → workflow検索・パース → runner実行
- `internal/config/` — `.flow.yaml` 設定ファイルの読み込み
- `internal/workflow/` — ワークフロー YAML のパース・バリデーション・検索
- `internal/runner/` — ワークフローの逐次実行（jobs → steps を順に `sh -c` で実行）
- `internal/version/` — version info injected at build time via ldflags (`Version`, `CommitSHA`, `BuildTime`)
- `.product_name` — contains the binary name (`flow`), read by Makefile
- `.goreleaser.yaml` — GoReleaser v2 config with ldflags for version injection
