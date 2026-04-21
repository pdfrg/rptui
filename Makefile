.PHONY: build build-release build-multiplatform install clean test run

VERSION ?= dev
BINARY := rptui
MAIN_PKG := ./cmd/rptui

# Last.fm API credentials - can be overridden or auto-loaded from .build-secrets
LASTFM_KEY ?=
LASTFM_SECRET ?=

# Build ldflags
LDFLAGS = -s -w -X main.Version=$(VERSION)

# If Last.fm keys provided (or found in .build-secrets), embed them
ifneq ($(LASTFM_KEY),)
LDFLAGS += -X github.com/pdfrg/rptui/internal/api.LastFMAPIKey=$(LASTFM_KEY)
LDFLAGS += -X github.com/pdfrg/rptui/internal/api.LastFMSharedSecret=$(LASTFM_SECRET)
endif

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(MAIN_PKG)

build-release:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(MAIN_PKG)

# Multi-platform build with Last.fm API keys embedded
# Usage:
#   make build-multiplatform                    # Uses .build-secrets if available
#   make build-multiplatform VERSION=v1.0.0   # Specify version
#   make build-multiplatform LASTFM_KEY=xxx LASTFM_SECRET=xxx VERSION=v1.0.0
build-multiplatform:
	@mkdir -p dist
	@echo "Building for linux/amd64..."
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/rptui-linux-amd64 $(MAIN_PKG)
	@echo "Building for linux/arm64..."
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/rptui-linux-arm64 $(MAIN_PKG)
	@echo "Building for darwin/amd64..."
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/rptui-darwin-amd64 $(MAIN_PKG)
	@echo "Building for darwin/arm64..."
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/rptui-darwin-arm64 $(MAIN_PKG)
	@echo "Building for windows/amd64..."
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/rptui.exe $(MAIN_PKG)
	@echo ""
	@echo "Done! Binaries built:"
	@ls -lh dist/

install:
	go install -ldflags "$(LDFLAGS)" $(MAIN_PKG)

clean:
	rm -f $(BINARY)
	rm -rf dist

test:
	go test ./...

run:
	go run $(MAIN_PKG)