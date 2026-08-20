.PHONY: dev dev-demo backend frontend lint test build build-linux-amd64 build-linux-arm64 tidy

BIN := backend/bin/homelab

# Run the full dev stack (backend in real mode + vite dev server)
dev:
	@echo "Starting backend (real mode) and frontend dev server..."
	@trap 'kill 0' INT TERM EXIT; \
	(cd backend && go run ./cmd/homelab --data-dir ../data 2>&1 | sed 's/^/[backend] /') & \
	(cd frontend && npm run dev 2>&1 | sed 's/^/[frontend] /') & \
	wait

# Run the full dev stack with the backend in deterministic demo mode
dev-demo:
	@echo "Starting backend (demo mode) and frontend dev server..."
	@trap 'kill 0' INT TERM EXIT; \
	(cd backend && go run ./cmd/homelab --demo --data-dir ../data 2>&1 | sed 's/^/[backend] /') & \
	(cd frontend && npm run dev 2>&1 | sed 's/^/[frontend] /') & \
	wait

backend:
	cd backend && go run ./cmd/homelab --data-dir ../data

frontend:
	cd frontend && npm run dev

lint:
	cd backend && gofmt -l . && go vet ./...
	cd frontend && npm run lint

test:
	cd backend && go test ./...
	cd frontend && npm run test

build: build-linux-arm64 build-linux-amd64

build-linux-amd64:
	cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o bin/homelab-amd64 ./cmd/homelab

build-linux-arm64:
	cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o bin/homelab-arm64 ./cmd/homelab

tidy:
	cd backend && go mod tidy