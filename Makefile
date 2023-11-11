
# Local config
CONTAINER_NAME=gateway
CONTAINER_PORT_HTTP=80
CONTAINER_PORT_PROM=2112
RELEASE_VERSION=0.0.1

REPO=cashtrack/gateway
IMAGE_RELEASE=$(REPO):$(RELEASE_VERSION)
IMAGE_DEV=$(REPO):dev
IMAGE_LATEST=$(REPO):latest

.PHONY: run build tag push start stop

run:
	go run main.go

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
      $(IMAGE_DEV)

stop:
	docker stop $(CONTAINER_NAME)
