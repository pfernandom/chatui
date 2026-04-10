# Repository Guidelines

## Project Structure & Module Organization

This repository is a small Go module centered on a reusable inline chat shell built on [github.com/pfernandom/go-tui](https://github.com/pfernandom/go-tui) (a fork of [grindlemire/go-tui](https://github.com/grindlemire/go-tui)).

- `main.go`: primary demo app using the reusable `chat` package
- `chat/`: reusable package code and tests
- `cmd/simple-chat/`, `cmd/simple-chat-2/`: alternative runnable examples
- `go.mod`, `go.sum`: module definition and dependencies
- `third_party/go-tui`: **git submodule** — [github.com/pfernandom/go-tui](https://github.com/pfernandom/go-tui) (fork). The root `go.mod` uses `replace github.com/pfernandom/go-tui => ./third_party/go-tui` so builds in this repo use the submodule. **Downstream modules that import `chatui` ignore that `replace`** and still resolve `go-tui` via the version in `require` (module proxy / `go get`).

Keep reusable behavior in `chat/`. Reserve `main.go` and `cmd/` programs for demos or integration examples.

## Dependencies

Do not commit a `vendor/` tree or loose copies of third-party module sources in the tree.

**Working in this repository:** after `git clone`, initialize the submodule:

```bash
git submodule update --init --recursive
```

Edit code under `third_party/go-tui`, commit the submodule pointer in the parent repo when you advance the fork, and bump the `require github.com/pfernandom/go-tui` version in `go.mod` when you want the declared version to match a tagged release of the fork (`go get github.com/pfernandom/go-tui@vx.y.z` updates the requirement; the `replace` continues to point at `./third_party/go-tui` for local builds).

**Consuming `chatui` as a dependency:** use `go get github.com/pfernandom/chatui@v…` as usual; your module does not need this submodule — Go uses the published `go-tui` version from `require` only.

## Build, Test, and Development Commands

- `go test ./...`: run the full test suite
- `go build ./...`: compile all packages and example binaries
- `go run .`: run the primary demo from the repository root
- `go run ./cmd/simple-chat`: run the first minimal example
- `go run ./cmd/simple-chat-2`: run the second minimal example
- `make docker-demo` or `./scripts/docker-demo.sh`: build and run the demo in Docker (**`-it` TTY required**). Optional `CHATUI_DOCKER_STRESS=1` exercises stream output plus `WriteElement` tool cards (similar to agent UIs).

If the local Go build cache is restricted, use:

```bash
env GOCACHE=/tmp/chatui-go-build-cache go test ./...
```

Docker builds need the submodule checked out before `docker build` (so `third_party/go-tui` exists in the build context). CI should run `git submodule update --init --recursive` first.

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

**Inline strip sizing:** `Config.CompactHeight` is the minimum height in rows (and the maximum when multiline mode is off). `Config.MultilineHeight` is the maximum height when multiline mode is on. The shell computes the actual inline height from padding, title, wrapped instructions/meta/status/slash hint, and the composer, then clamps to those bounds and to the terminal height.
