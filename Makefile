IMAGE_NAME ?= ghcr.io/albinoanteater/cert-manager-webhook-namedotcom
IMAGE_TAG  ?= dev

.PHONY: build test vet docker-build

build:
	go build ./...

vet:
	go vet ./...

test:
	go test -v -run TestHelpers ./...

conformance-test:
	TEST_ZONE_NAME=$(TEST_ZONE_NAME) go test -v -tags conformance ./...

docker-build:
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) .
