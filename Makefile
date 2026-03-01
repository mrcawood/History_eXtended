# hx build

.PHONY: build install test test-sync clean

build:
	go build -tags sqlite_fts5 -o bin/hx ./cmd/hx
	go build -o bin/hx-emit ./cmd/hx-emit
	go build -tags sqlite_fts5 -o bin/hxd ./cmd/hxd

install: build
	mkdir -p $(HOME)/.local/bin
	cp bin/hx bin/hx-emit bin/hxd $(HOME)/.local/bin/
	@echo "Installed hx, hx-emit, hxd to $(HOME)/.local/bin"
	@echo "Add $(HOME)/.local/bin to PATH if needed."
	@echo "Source src/hooks/hx.zsh from .zshrc to enable capture."

test:
	go test ./...

test-s3-integration:
	@if ! command -v docker >/dev/null 2>&1; then \
		echo "Docker is required for S3 integration tests"; \
		exit 1; \
	fi
	@echo "Starting MinIO for integration tests..."
	docker run -d --name hx-minio-test -p 9000:9000 -p 9001:9001 \
		-e "MINIO_ROOT_USER=minioadmin" \
		-e "MINIO_ROOT_PASSWORD=minioadmin" \
		minio/minio server /data --console-address ":9001"
	@echo "Waiting for MinIO to start..."
	@sleep 5
	@echo "Creating test bucket..."
	@docker exec hx-minio-test mc alias set myminio http://localhost:9000 minioadmin minioadmin
	@docker exec hx-minio-test mc mb myminio/hx-test || true
	@echo "Running S3 integration tests..."
	HX_REQUIRE_S3_ENDPOINT=1 go test ./internal/sync/... -v -run "TestTwoNodeConverge_MinIO|TestTombstonePropagation_MinIO|TestCorruptManifest_MinIO|TestEfficiency_ManifestReducesListCalls|TestRetryableStore_MinIOIntegration"
	@echo "Cleaning up MinIO..."
	docker stop hx-minio-test
	docker rm hx-minio-test

test-sync:
	go test -tags sqlite_fts5 ./internal/sync/... -v -timeout 30s

clean:
	rm -rf bin/
