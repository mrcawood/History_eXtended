# hx build

.PHONY: build install test test-sync clean

build:
	go build -tags sqlite_fts5 -o bin/hx ./cmd/hx
	go build -o bin/hx-emit ./cmd/hx-emit
	go build -tags sqlite_fts5 -o bin/hxd ./cmd/hxd

HX_LIB_DIR = $(HOME)/.local/lib/hx

install: build
	mkdir -p $(HOME)/.local/bin
	install -m 755 bin/hx bin/hx-emit bin/hxd $(HOME)/.local/bin/
	mkdir -p $(HX_LIB_DIR)
	install -m 644 src/hooks/hx.zsh src/hooks/bash/hx.bash $(HX_LIB_DIR)/
	install -m 755 scripts/start-hxd-if-needed.sh $(HX_LIB_DIR)/
	@echo ""
	@echo "========================================"
	@echo "  Installed to $(HOME)/.local/bin"
	@echo "  Hooks to $(HX_LIB_DIR)"
	@echo "----------------------------------------"
	@if [ -f "$${XDG_DATA_HOME:-$$HOME/.local/share}/hx/hxd.pid" ] 2>/dev/null; then \
		pid=$$(cat "$${XDG_DATA_HOME:-$$HOME/.local/share}/hx/hxd.pid" 2>/dev/null); \
		if kill -0 "$$pid" 2>/dev/null; then \
			echo "  • Restarting hxd (pid $$pid) to use new binary..."; \
			kill "$$pid" 2>/dev/null; \
			for i in 1 2 3 4 5; do kill -0 "$$pid" 2>/dev/null || break; sleep 1; done; \
			if kill -0 "$$pid" 2>/dev/null; then \
				echo "  • WARN: hxd did not exit; run 'kill -9 $$pid' if needed"; \
			elif [ -x "$(HOME)/.local/bin/hxd" ]; then \
				"$(HOME)/.local/bin/hxd" & \
				echo "  • hxd restarted."; \
			fi; \
		fi; \
	fi
	@sh "$(CURDIR)/scripts/install-path.sh"
	@sh "$(CURDIR)/scripts/install-hook.sh" "$(HX_LIB_DIR)"
	@sh "$(CURDIR)/scripts/install-daemon.sh" "$(HX_LIB_DIR)"
	@echo "========================================"

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
