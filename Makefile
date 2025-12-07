.PHONY: all build run dev clean docker-build docker-up docker-down test test-backend test-frontend test-coverage

# Variables
BACKEND_DIR = backend
FRONTEND_DIR = frontend
BINARY_NAME = chatapp-server

# Default target
all: build

# Generate default config
config:
	cd $(BACKEND_DIR) && go run ./cmd/server -generate-config
	@echo "Generated config.json - please add your API keys"

# Build everything
build: build-backend build-frontend

# Build backend
build-backend:
	cd $(BACKEND_DIR) && go build -o ../bin/$(BINARY_NAME) ./cmd/server

# Build frontend
build-frontend:
	cd $(FRONTEND_DIR) && npm install && npm run build

# Run backend in development mode
dev-backend:
	cd $(BACKEND_DIR) && go run ./cmd/server

# Run frontend in development mode
dev-frontend:
	cd $(FRONTEND_DIR) && npm install && npm run dev

# Run both in development (requires tmux or run in separate terminals)
dev:
	@echo "Run 'make dev-backend' and 'make dev-frontend' in separate terminals"

# Run production build
run: build
	./bin/$(BINARY_NAME)

# Clean build artifacts
clean:
	rm -rf bin/
	rm -rf $(FRONTEND_DIR)/dist
	rm -rf $(FRONTEND_DIR)/node_modules

# Docker commands
docker-build:
	docker-compose build

docker-up:
	docker-compose up -d

docker-down:
	docker-compose down

docker-logs:
	docker-compose logs -f

# Production with nginx
docker-prod:
	docker-compose --profile production up -d

# Install dependencies
deps:
	cd $(BACKEND_DIR) && go mod download
	cd $(FRONTEND_DIR) && npm install

# Format code
fmt:
	cd $(BACKEND_DIR) && go fmt ./...

# Run all tests
test: test-backend test-frontend

# Run backend tests
test-backend:
	@echo "Running Go backend tests..."
	cd $(BACKEND_DIR) && go test ./... -v

# Run frontend tests
test-frontend:
	@echo "Running Vue frontend tests..."
	cd $(FRONTEND_DIR) && npm run test

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	cd $(BACKEND_DIR) && go test ./... -cover
	cd $(FRONTEND_DIR) && npm run test:coverage

# Help
help:
	@echo "Available targets:"
	@echo "  config        - Generate default config file"
	@echo "  build         - Build backend and frontend"
	@echo "  build-backend - Build Go backend only"
	@echo "  build-frontend- Build Vue frontend only"
	@echo "  dev-backend   - Run backend in dev mode"
	@echo "  dev-frontend  - Run frontend in dev mode (with hot reload)"
	@echo "  run           - Build and run production server"
	@echo "  clean         - Remove build artifacts"
	@echo "  docker-build  - Build Docker images"
	@echo "  docker-up     - Start Docker containers"
	@echo "  docker-down   - Stop Docker containers"
	@echo "  docker-prod   - Start with nginx reverse proxy"
	@echo "  deps          - Install all dependencies"
	@echo "  test          - Run all tests (backend + frontend)"
	@echo "  test-backend  - Run Go backend tests"
	@echo "  test-frontend - Run Vue frontend tests"
	@echo "  test-coverage - Run tests with coverage reports"
