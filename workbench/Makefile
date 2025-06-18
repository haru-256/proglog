ifndef GOARCH
	GOARCH := $(shell go env GOARCH)
endif

ifndef GOOS
	GOOS := $(shell go env GOOS)
endif

.DEFAULT_GOAL := help
ifndef CONFIG_PATH
	CONFIG_PATH := ./.config
endif

.PHONY: init
init:
	mkdir -p $(CONFIG_PATH)

$(CONFIG_PATH)/model.conf:
	cp test/model.conf $(CONFIG_PATH)/model.conf

$(CONFIG_PATH)/policy.csv:
	cp test/policy.csv $(CONFIG_PATH)/policy.csv

.PHONY: test
test: $(CONFIG_PATH)/policy.csv $(CONFIG_PATH)/model.conf ## Execute tests
	go test -race -shuffle=on ./...

.PHONY: lint
lint: ## Run Linter
	golangci-lint run --config=./.golangci.yml ./...

.PHONY: gencert
gencert: ## Generate self-signed certificate
	# *.csr: Certificate Signing Request
	# *.pem: Certificate
	# *.key: Private Key
	cfssl gencert \
		-initca test/ca-csr.json | cfssljson -bare ca
	cfssl gencert \
		-ca=ca.pem \
		-ca-key=ca-key.pem \
		-config=test/ca-config.json \
		-profile=server \
		test/server-csr.json | cfssljson -bare server
	cfssl gencert \
		-ca=ca.pem \
		-ca-key=ca-key.pem \
		-config=test/ca-config.json \
		-profile=client \
		-cn="root" \
		test/client-csr.json | cfssljson -bare root-client
	cfssl gencert \
		-ca=ca.pem \
		-ca-key=ca-key.pem \
		-config=test/ca-config.json \
		-profile=client \
		-cn="nobody" \
		test/client-csr.json | cfssljson -bare nobody-client
	mv *.pem *.csr $(CONFIG_PATH)

.PHONY: fmt
fmt: ## Run formatter
	go fmt ./...

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
	go install github.com/cloudflare/cfssl/cmd/cfssl@latest
	go install github.com/cloudflare/cfssl/cmd/cfssljson@v1.6.1

.PHONY: build
build: ## Build project
	CGO_ENABLED=0 GO111MODULE=on GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o ./bin/main


.PHONY: help
help: ## Show options
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
