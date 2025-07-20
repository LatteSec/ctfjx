BINARY_NAME := ctfjx
BIN_DIR     := bin
VERSION      = devel
BUILD_FLAGS := -ldflags="-s -w -X github.com/lattesec/ctfjx/version.Version=$(VERSION)"

# Binaries
CTFX  := ctfjx
CTFXD := ctfjxd
WEB   := ctfjx-web
AGENT := ctfjx-agent

ifeq ($(OS),Windows_NT)
RM_CMD:=rd /s /q
NULL:=/dev/nul
EXT:=.exe
else
RM_CMD:=rm -rf
NULL:=/dev/null
EXT=
endif



# =================================== DEFAULT =================================== #

default: all

## default: Runs build and test
.PHONY: default
all: build test

# =================================== HELPERS =================================== #

## help: print this help message
.PHONY: help
help:
	@echo 'CTFx - A CTF hosting platform'
	@echo ''
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Commands:'
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':' |  sed -e 's/^/ /'
	@echo ''
	@echo 'Extra:'
	@sed -n 's/^### //p' $(MAKEFILE_LIST) | column -t -s ':' |  sed -e 's/^/ /'




## install: Install dependencies
.PHONY: install
install:
	go get ./...

# =================================== DEVELOPMENT =================================== #

## docs: Runs Documentation
.PHONY: docs
docs:
	mkdocs serve -f www/mkdocs.yml -a 0.0.0.0:8000




## build: Builds Go binary
.PHONY: build
build: build/$(CTFX) build/$(CTFXD) build/$(WEB) build/$(AGENT)

.PHONY: build/$(CTFX)
build/$(CTFX):
	go build $(BUILD_FLAGS) -o $(BIN_DIR)/$(CTFX)$(EXT) ./cmd/v1/ctfjx

.PHONY: build/$(CTFXD)
build/$(CTFXD):
	 go build $(BUILD_FLAGS) -o $(BIN_DIR)/$(CTFXD)$(EXT) ./cmd/v1/ctfjxd

.PHONY: build/$(WEB)
build/$(WEB):
	go build $(BUILD_FLAGS) -o $(BIN_DIR)/$(WEB)$(EXT) ./cmd/v1/ctfjx-web

.PHONY: build/$(AGENT)
build/$(AGENT):
	go build $(BUILD_FLAGS) -o $(BIN_DIR)/$(AGENT)$(EXT) ./cmd/v1/ctfjx-agent

## build/docs: Builds documentation
build/docs:
	mkdocs build -f www/mkdocs.yml

### build/docker: Builds Docker image
build/docker:
	docker build -t $(WEB):$(TAG) -f Dockerfile .




## test: Runs tests
.PHONY: test
test:
	go mod tidy
	go mod verify
	go vet ./...
	go test -race ./...




## bench: Run benchmarks
bench:
	go test -v -bench=. -benchmem ./...




# =================================== QUALITY ================================== #

## lint: Lint code
.PHONY: lint
lint: lint/go lint/npm

### lint/go: Lint Go code
.PHONY: lint/go
lint/go:
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run

### lint/npm: Lint NPM code
.PHONY: lint/npm
lint/npm:
	prettier --cache --check .



## security: Run security checks
.PHONY: security
security:
	go run github.com/securego/gosec/v2/cmd/gosec@latest -quiet ./...
	go run github.com/go-critic/go-critic/cmd/gocritic@latest check -enableAll ./...
	go run github.com/google/osv-scanner/cmd/osv-scanner@latest -r .




## format: Format code
.PHONY: format
format: format/go format/npm

### format/go: Format Go code
.PHONY: format/go
format/go:
	go fmt ./...
	go mod tidy -v
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run --fix

### format/npm: Format NPM code
.PHONY: format/npm
format/npm:
	prettier --cache --write .




## tidy: Clean up code artifacts
.PHONY: tidy
tidy:
	go clean ./...
	$(RM_CMD) $(BIN_DIR)




## clean: Remove node_modules
.PHONY: clean
clean: tidy
	$(RM_CMD) node_modules
