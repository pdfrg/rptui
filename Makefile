.PHONY: build build-release install clean test run

VERSION ?= dev
BINARY := rptui
MAIN_PKG := ./cmd/rptui

# Last.fm API credentials (for release builds)
# These can be overridden: make build-release LASTFM_KEY=xxx LASTFM_SECRET=xxx
LASTFM_KEY ?= 
LASTFM_SECRET ?= 

build:
	go build -ldflags "-s -w -X main.Version=$(VERSION)" -o $(BINARY) $(MAIN_PKG)

build-release:
	go build -ldflags "-s -w -X main.Version=$(VERSION) -X github.com/pdfrg/rptui/internal/api.LastFMAPIKey=$(LASTFM_KEY) -X github.com/pdfrg/rptui/internal/api.LastFMSharedSecret=$(LASTFM_SECRET)" -o $(BINARY) $(MAIN_PKG)

install:
	go install -ldflags "-s -w -X main.Version=$(VERSION)" $(MAIN_PKG)

clean:
	rm -f $(BINARY)

test:
	go test ./...

run:
	go run $(MAIN_PKG)
