# Interactive TTY demo for narrow-terminal / Docker validation (docker run -it required).
# Requires third_party/go-tui (git submodule) in the build context — run
# `git submodule update --init --recursive` before `docker build`.
# syntax=docker/dockerfile:1

FROM golang:1.25-bookworm AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/chatui-demo .

FROM debian:bookworm-slim
RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates \
	&& rm -rf /var/lib/apt/lists/* \
	&& useradd -m -u 1000 demo
COPY --from=builder /out/chatui-demo /usr/local/bin/chatui-demo
USER demo
WORKDIR /home/demo
ENV TERM=xterm-256color
ENTRYPOINT ["/usr/local/bin/chatui-demo"]
