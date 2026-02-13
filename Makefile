# hx build

.PHONY: build install clean

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

clean:
	rm -rf bin/
