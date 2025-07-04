APP_NAME = rpkirtr2
BIN_DIR = bin
SRC_DIRS = ./cmd/ ./internal/

.PHONY: all build clean test fmt vet test-cover test-race

all: build

build:
	@echo "Building $(APP_NAME)..."
	@go build -o $(BIN_DIR)/$(APP_NAME) ./cmd/$(APP_NAME)/

run: build
	@echo "Running $(APP_NAME)..."
	@./$(BIN_DIR)/$(APP_NAME)

test:
	@echo "Running tests..."
	@go test -v ./...

test-cover:
	@echo "Running tests with coverage report..."
	@go test -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out

test-race:
	@echo "Running tests with race detector enabled..."
	@go test -race -v ./...

fmt:
	@echo "Formatting code..."
	@gofmt -w $(SRC_DIRS)

vet:
	@echo "Vet checking..."
	@go vet ./...

clean:
	@echo "Cleaning up..."
	@rm -rf $(BIN_DIR) coverage.out
