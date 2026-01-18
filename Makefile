.PHONY: build run test clean

BINARY=xhub
BUILD_DIR=bin
CGO_FLAGS=-tags "fts5"

build:
	CGO_ENABLED=1 go build $(CGO_FLAGS) -o $(BUILD_DIR)/$(BINARY) .

run: build
	./$(BUILD_DIR)/$(BINARY)

test:
	CGO_ENABLED=1 go test $(CGO_FLAGS) -v ./...

clean:
	rm -rf $(BUILD_DIR)

install: build
	cp $(BUILD_DIR)/$(BINARY) /usr/local/bin/$(BINARY)
