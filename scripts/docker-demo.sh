#!/usr/bin/env bash
# Build and run the inline chat demo in Docker with a TTY (required for go-tui).
# Usage:
#   ./scripts/docker-demo.sh
# Agent-like stress (stream + WriteElement tool card):
#   CHATUI_DOCKER_STRESS=1 ./scripts/docker-demo.sh
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
docker build -t chatui-demo:local .
exec docker run --rm -it \
	-e CHATUI_DOCKER_STRESS="${CHATUI_DOCKER_STRESS:-}" \
	chatui-demo:local
