BINARY     := workpulse
MODULE     := github.com/juanpabloaj/workpulse
CMD        := ./cmd/workpulse

VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS    := -ldflags "-X main.version=$(VERSION) -X main.buildDate=$(BUILD_DATE)"

.PHONY: build run test fmt lint clean install

build:
	go build $(LDFLAGS) -o $(BINARY) $(CMD)

run:
	go run $(CMD)

test:
	go test ./...

fmt:
	gofmt -w ./cmd ./internal

lint:
	go vet ./...

clean:
	rm -f $(BINARY)

install:
	go install $(LDFLAGS) $(CMD)
