# stompbox — build and install the `stomp` CLI.

# Install location. Override on the command line, e.g.:
#   make install PREFIX=$HOME/.local      (user install, no sudo)
#   sudo make install                     (system install in /usr/local)
PREFIX  ?= /usr/local
DESTDIR ?=
BINDIR  := $(DESTDIR)$(PREFIX)/bin

BIN  := stomp
PKG  := ./cmd/stomp

# Stamp the binary with the current git description when available.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

# Resolve the Go toolchain. Falls back to the standard tarball location
# (/usr/local/go/bin/go) so `sudo make install` works even when that dir is
# not on root's PATH. Override with `make install GO=/path/to/go` if needed.
GO ?= $(shell command -v go 2>/dev/null || command -v /usr/local/go/bin/go 2>/dev/null || echo go)

.PHONY: all build install uninstall fmt vet test check clean

all: build

build: ## Build the stomp binary in the repo root
	$(GO) build -ldflags '$(LDFLAGS)' -o $(BIN) $(PKG)

install: ## Build and install stomp into $(BINDIR)
	@command -v $(GO) >/dev/null 2>&1 || { \
		echo "error: '$(GO)' not found in PATH."; \
		echo "  If you used 'sudo make install', Go is probably missing from root's PATH."; \
		echo "  Install locally instead (no sudo needed, recommended for a user tool):"; \
		echo "      make install PREFIX=\$$HOME/.local"; \
		echo "  Or keep sudo and pass Go's full path:"; \
		echo "      sudo make install GO=\$$(command -v go)"; \
		exit 1; }
	@install -d $(BINDIR) 2>/dev/null || true
	@test -w $(BINDIR) || { \
		echo "error: cannot write to $(BINDIR) (permission denied)."; \
		echo "  Install locally instead (no sudo needed, recommended for a user tool):"; \
		echo "      make install PREFIX=\$$HOME/.local"; \
		echo "      # then make sure \$$HOME/.local/bin is on your PATH"; \
		echo "  Or install system-wide with sudo (keeping your Go on PATH):"; \
		echo "      sudo make install GO=\$$(command -v go)"; \
		exit 1; }
	$(GO) build -ldflags '$(LDFLAGS)' -o $(BINDIR)/$(BIN) $(PKG)
	@echo "installed $(BINDIR)/$(BIN)"

uninstall: ## Remove the installed binary
	rm -f $(BINDIR)/$(BIN)

fmt: ## Check formatting (matches CI)
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "Unformatted files (run 'gofmt -w .'):"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

vet: ## Run go vet
	$(GO) vet ./...

test: ## Run tests
	$(GO) test ./...

check: fmt vet test ## Run all CI checks

clean: ## Remove build artifacts
	rm -f $(BIN)
