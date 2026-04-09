# Repository Guidelines

## Project Structure & Module Organization

This repository is a small Go module centered on a reusable inline chat shell built on [github.com/pfernandom/go-tui](https://github.com/pfernandom/go-tui) (a fork of [grindlemire/go-tui](https://github.com/grindlemire/go-tui)).

- `main.go`: primary demo app using the reusable `chat` package
- `chat/`: reusable package code and tests
- `cmd/simple-chat/`, `cmd/simple-chat-2/`: alternative runnable examples
- `go.mod`, `go.sum`: module definition and dependencies

Keep reusable behavior in `chat/`. Reserve `main.go` and `cmd/` programs for demos or integration examples.

## Dependencies

Do not commit a `vendor/` tree or loose copies of third-party module sources in the tree.

Terminal UI comes from **`github.com/pfernandom/go-tui`** as a normal module dependency (`go.mod` / `go.sum`). Consumers only need `go mod download`; no `replace` and no submodule. Fixes and releases are developed in the fork repository; bump the version here with `go get github.com/pfernandom/go-tui@vx.y.z` (or `@main` / a commit pseudo-version while iterating).

## Build, Test, and Development Commands

- `go test ./...`: run the full test suite
- `go build ./...`: compile all packages and example binaries
- `go run .`: run the primary demo from the repository root
- `go run ./cmd/simple-chat`: run the first minimal example
- `go run ./cmd/simple-chat-2`: run the second minimal example

If the local Go build cache is restricted, use:

```bash
env GOCACHE=/tmp/chatui-go-build-cache go test ./...
```

## Coding Style & Naming Conventions

Use standard Go formatting and idioms.

- Format with `gofmt -w <file>` before finishing changes
- Use tabs for indentation, as produced by `gofmt`
- Exported names use `CamelCase`; unexported helpers use `camelCase`
- Keep public API additions in `chat/` small and explicit
- Prefer behavior-oriented names like `HandleResponse`, `SetStatus`, `renderOverlay`

## Testing Guidelines

Tests use Go’s built-in `testing` package and live next to the code in `*_test.go` files.

- Name tests `TestXxx`
- Add focused unit tests for public API behavior and request lifecycle changes
- Run `go test ./...` after every change
- When adding shell behavior, cover cancellation, status updates, and default/fallback behavior

## Commit & Pull Request Guidelines

Git history is not available in this checkout, so follow a simple imperative style:

- `chat: add request-scoped status override`
- `main: update demo to show custom status`

For pull requests, include:

- a short summary of user-visible behavior changes
- notes on API changes in `chat/`
- test results (`go test ./...`)
- screenshots or terminal recordings when UI behavior changes materially

## Architecture Notes

`chat` owns the inline-mode shell, request lifecycle, streaming, and status line behavior. Callers provide response handlers and optional overlay views; demos should consume that API rather than duplicating shell logic.
