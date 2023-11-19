
# Local config
CONTAINER_NAME=cashtrack_gateway
CONTAINER_PORT_HTTP=8081
RELEASE_VERSION=0.0.1

REPO=cashtrack/gateway
IMAGE_RELEASE=$(REPO):$(RELEASE_VERSION)
IMAGE_DEV=$(REPO):dev
IMAGE_LATEST=$(REPO):latest

.PHONY: run test build tag push start stop

run:
	go run -race main.go

test:
	go test -race -v ./...

build:
	docker build . -t $(IMAGE_DEV)

tag:
	docker tag $(IMAGE_DEV) $(IMAGE_RELEASE)
	docker tag $(IMAGE_DEV) $(IMAGE_LATEST)

push:
	docker push $(IMAGE_RELEASE)
	docker push $(IMAGE_LATEST)

start:
	docker run \
      --rm \
      --name $(CONTAINER_NAME) \
      -p $(CONTAINER_PORT_HTTP):80 \
      --env-file .env \
      -e HTTPS_ENABLED=false \
      --net cash-track-local \
      $(IMAGE_DEV)

stop:
	docker stop $(CONTAINER_NAME)
