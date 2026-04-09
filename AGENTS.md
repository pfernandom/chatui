# Repository Guidelines

## Project Structure & Module Organization

This repository is a small Go module centered on a reusable inline chat shell built on `go-tui`.

- `main.go`: primary demo app using the reusable `chat` package
- `chat/`: reusable package code and tests
- `cmd/simple-chat/`, `cmd/simple-chat-2/`: alternative runnable examples
- `go.mod`, `go.sum`: module definition and dependencies
- `deps/go-tui`: git submodule ([github.com/pfernandom/go-tui](https://github.com/pfernandom/go-tui)), a fork of `github.com/grindlemire/go-tui` with any chatui-specific fixes

Keep reusable behavior in `chat/`. Reserve `main.go` and `cmd/` programs for demos or integration examples.

## Dependencies

Do not commit a `vendor/` tree or loose copies of third-party module sources in the tree.

`go-tui` is consumed via a **git submodule** at `deps/go-tui` and a `replace` in `go.mod` that points `github.com/grindlemire/go-tui` at that path (import paths stay `github.com/grindlemire/go-tui`). After cloning chatui, run:

```bash
git submodule update --init --recursive
```

Changes to the fork are made inside `deps/go-tui`, committed there, and pushed to [pfernandom/go-tui](https://github.com/pfernandom/go-tui); the parent repo records the submodule commit SHA. The fork keeps `module github.com/grindlemire/go-tui`, so imports stay unchanged and the `replace` directive is the supported way to build this repo. Tag the fork when you want a named revision (for example after merging fixes).

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
