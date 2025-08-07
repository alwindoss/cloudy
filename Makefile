BINARY_LOC=bin
BINARY_NAME=cloudy
DOCKER_REPOSITORY_OWNER=alwindoss
VERSION=0.0.5

.PHONY: help build run package deploy tag-latest

help:
	@echo "Use make build command"

build:
	go build -o ./$(BINARY_LOC)/ -v ./cmd/$(BINARY_NAME)/...

run: build
	./$(BINARY_LOC)/$(BINARY_NAME)

package:
	docker build -t $(DOCKER_REPOSITORY_OWNER)/$(BINARY_NAME):$(VERSION)  .

tag-latest:
	docker tag $(DOCKER_REPOSITORY_OWNER)/$(BINARY_NAME):$(VERSION) $(DOCKER_REPOSITORY_OWNER)/$(BINARY_NAME):latest

deploy: package tag-latest
	docker push $(DOCKER_REPOSITORY_OWNER)/$(BINARY_NAME):$(VERSION)
	docker push $(DOCKER_REPOSITORY_OWNER)/$(BINARY_NAME):latest