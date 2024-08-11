include .env
export

# Local config
CONTAINER_NAME=cashtrack_gateway
CONTAINER_PORT_HTTP=8081
RELEASE_VERSION=0.0.1

REPO=cashtrack/gateway
IMAGE_RELEASE=$(REPO):$(RELEASE_VERSION)
IMAGE_DEV=$(REPO):dev
IMAGE_LATEST=$(REPO):latest

.PHONY: run test build tag push start stop mock-gen

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

mock-gen:
	go install go.uber.org/mock/mockgen@latest
	mockgen -source=http/client.go -package=httpmock -destination=mocks/http/client_mock.go -mock_names=Client=ClientMock
	mockgen -source=http/retryhttp/client.go -package=mocks -destination=mocks/http_retry_client_mock.go -mock_names=Client=HttpRetryClientMock
	mockgen -source=captcha/provider.go -package=mocks -destination=mocks/captcha_provider_mock.go -mock_names=Provider=CaptchaProviderMock
	mockgen -source=service/api/service.go -package=mocks -destination=mocks/api_service_mock.go -mock_names=Service=ApiServiceMock
	mockgen -source=router/api/handler.go -package=mocks -destination=mocks/api_handler_mock.go -mock_names=Handler=ApiHandlerMock

