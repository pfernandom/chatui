.PHONY: docker-demo test build submodules

submodules:
	git submodule update --init --recursive

test:
	go test ./...

build:
	go build -o /dev/null .

# Requires Docker; allocates a TTY (-it). Optional: CHATUI_DOCKER_STRESS=1 make docker-demo
docker-demo:
	docker build -t chatui-demo:local .
	docker run --rm -it -e CHATUI_DOCKER_STRESS=$(CHATUI_DOCKER_STRESS) chatui-demo:local
