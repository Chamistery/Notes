BIN := bin/notes
GOBIN := $(shell go env GOPATH)/bin
export PATH := $(GOBIN):$(PATH)

.PHONY: build build-clients run test proto tidy clean tools

build:
	go build -o $(BIN) ./cmd/notes

build-clients:
	go build -o bin/h3client ./cmd/h3client
	go build -o bin/grpcclient ./cmd/grpcclient

run: build
	./$(BIN)

test:
	go test ./...

proto:
	protoc -I api/proto \
		--go_out=api/gen --go_opt=paths=source_relative \
		--go-grpc_out=api/gen --go-grpc_opt=paths=source_relative \
		api/proto/notes.proto

tools:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

tidy:
	go mod tidy

clean:
	rm -rf bin
