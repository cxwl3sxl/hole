GO ?= go
GOFLAGS ?= -ldflags="-s -w"
BINDIR ?= ./bin

.PHONY: all build server client clean test

all: build

build: server client

server:
	$(GO) build $(GOFLAGS) -o $(BINDIR)/whd ./cmd/server

client:
	$(GO) build $(GOFLAGS) -o $(BINDIR)/wh ./cmd/client

test:
	$(GO) test ./... -v

clean:
	rm -rf $(BINDIR)
