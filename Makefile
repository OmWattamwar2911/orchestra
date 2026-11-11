.PHONY: all proto build test test-race lint run-etcd up down clean

PROTO_DIR := api/proto
PROTO_FILES := $(PROTO_DIR)/orchestra/v1/orchestra.proto
BIN_DIR := bin

ifeq ($(OS),Windows_NT)
	BINARY_EXT := .exe
else
	BINARY_EXT :=
endif

all: build

proto:
	protoc --proto_path=$(PROTO_DIR) --go_out=. --go_opt=module=orchestra --go-grpc_out=. --go-grpc_opt=module=orchestra $(PROTO_FILES)

build: proto
	go build -o $(BIN_DIR)/orchestrator$(BINARY_EXT) ./cmd/orchestrator
	go build -o $(BIN_DIR)/worker$(BINARY_EXT) ./cmd/worker
	go build -o $(BIN_DIR)/orchctl$(BINARY_EXT) ./cmd/orchctl
	go build -o $(BIN_DIR)/chaos$(BINARY_EXT) ./cmd/chaos
	go build -o $(BIN_DIR)/checker$(BINARY_EXT) ./cmd/checker

test:
	go test -v ./...

test-race:
	go test -race -v ./...

lint:
	go vet ./...

run-etcd:
	docker run -d --name orchestra-etcd -p 2379:2379 -p 2380:2380 quay.io/coreos/etcd:v3.5.14 /usr/local/bin/etcd --listen-client-urls http://0.0.0.0:2379 --advertise-client-urls http://localhost:2379

up:
	docker compose -f deploy/docker-compose.yml up -d --build

down:
	docker compose -f deploy/docker-compose.yml down -v

clean:
	go clean
