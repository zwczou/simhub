GOPATH:=$(shell go env GOPATH)
API_PROTO_FILES=$(shell find apis -path apis/third_party -prune -o -name *.proto -print)


# init env
init:
	go get -tool google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go get -tool google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go get -tool github.com/envoyproxy/protoc-gen-validate@latest
	go get -tool github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@latest
	go get -tool github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest
	go get -u google.golang.org/grpc
	go get -u google.golang.org/protobuf
	go mod tidy
	go install tool

# generate protobuf
api:
	protoc --proto_path=. \
		--proto_path=./apis/third_party \
		--go_out=paths=source_relative:. \
		--go-grpc_out=paths=source_relative,require_unimplemented_servers=false:. \
		--grpc-gateway_out=paths=source_relative,logtostderr=true:. \
		--validate_out=paths=source_relative,lang=go:. \
		--openapiv2_out=use_go_templates=true,json_names_for_fields=false,omit_enum_default_value=true,disable_default_errors=true:. \
		$(API_PROTO_FILES)

# build simd
simd:
	go build -o build/apps/simd ./cmd/simd/

# build simd for linux
simd_linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o build/apps/simd_linux ./cmd/simd/


# build all
all: api simd

# build for linux
linux: api simd_linux

# clean
clean:
	@echo 'clean ...'
	@find apis build/apps -type f -not -name "*.proto" | xargs -ti rm {}
	@echo 'clean OK'

# show help message
help:
	@echo ''
	@echo 'Usage:'
	@echo ' make [target]'
	@echo ''
	@echo 'Targets:'
	@awk '/^[a-zA-Z\-\_0-9]+:/ { \
	helpMessage = match(lastLine, /^# (.*)/); \
		if (helpMessage) { \
			helpCommand = substr($$1, 0, index($$1, ":")-1); \
			helpMessage = substr(lastLine, RSTART + 2, RLENGTH); \
			printf "\033[36m%-32s\033[0m %s\n", helpCommand,helpMessage; \
		} \
	} \
	{ lastLine = $$0 }' $(MAKEFILE_LIST)

.DEFAULT_GOAL := help

.PHONY: init api simd simd_linux linux clean help
