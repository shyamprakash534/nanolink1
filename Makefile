.PHONY: all build test run-go run-engine docker-up docker-down terraform-init clean

all: test build

build:
	@echo "Building NanoLink Go binary..."
	CGO_ENABLED=0 go build -ldflags="-w -s" -o bin/nanolink ./cmd/server

test:
	@echo "Running test suite..."
	cd engine && python3 test_nanolink.py

benchmark:
	@echo "Running performance benchmarks..."
	cd engine && python3 benchmark.py

run-engine:
	@echo "Starting NanoLink standalone engine on http://localhost:8080..."
	cd engine && python3 main.py

docker-up:
	@echo "Starting full distributed stack with Docker Compose..."
	docker compose up --build -d

docker-down:
	@echo "Stopping distributed stack..."
	docker compose down -v

terraform-init:
	@echo "Initializing Terraform AWS configurations..."
	cd terraform && terraform init

clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/ /tmp/nanolink_data.db
