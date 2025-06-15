ifndef GOARCH
	GOARCH=$(shell go env GOARCH)
endif

ifndef GOOS
	GOOS := $(shell go env GOOS)
endif

.DEFAULT_GOAL := help

.PHONY: test
test: ## Execute tests
	go test -race -shuffle=on ./...

.PHONY: lint
lint: ## Run Linter
	golangci-lint run --config=./.golangci.yml ./...

.PHONY: fmt
fmt: ## Run formatter
	go fmt ./...

.PHONY: init
init: ## Initialize project
	cobra-cli init

.PHONY: compile
compile: ## Generate protobuf files
	protoc api/v1/*.proto \
		--go_out=. \
		--go-grpc_out=. \
		--go_opt=paths=source_relative \
		--go-grpc_opt=paths=source_relative \
		--proto_path=.

.PHONY: install
install: ## Install dependencies
	go mod tidy
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

.PHONY: build
build: ## Build project
	CGO_ENABLED=0 GO111MODULE=on GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o ./bin/main


.PHONY: help
help: ## Show options
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
