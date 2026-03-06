MODULE := github.com/nealhardesty/easy-llm-wrapper
BIN    := elw

.PHONY: build build-elw install-elw test test-unit test-functional lint fmt tidy clean version version-increment push help

## build: Compile the library and CLI tool
build:
	go build ./...

## build-elw: Build the elw CLI binary into the project root
build-elw:
	go build -o $(BIN) ./cmd/elw

## install-elw: Install the elw CLI to GOPATH/bin
install-elw:
	go install ./cmd/elw

## build-elwi: Build the elwi image generation CLI binary into the project root
build-elwi:
	go build -o elwi ./cmd/elwi

## install-elwi: Install the elwi CLI to GOPATH/bin
install-elwi:
	go install ./cmd/elwi

## test: Run all unit tests with race detector
test:
	go test -race ./...

## test-unit: Alias for test (unit tests only, no functional tag)
test-unit: test

## test-functional: Run functional/integration tests (requires OLLAMA_HOST or OPENROUTER_API_KEY)
test-functional:
	go test -race -tags functional ./...

## lint: Run static analysis
lint:
	go vet ./...
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed, skipping (install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)"; \
	fi

## fmt: Format code with gofmt (and goimports if available)
fmt:
	gofmt -w .
	@if command -v goimports >/dev/null 2>&1; then \
		goimports -w .; \
	fi

## tidy: Clean up go.mod and go.sum
tidy:
	go mod tidy

## clean: Remove build artifacts and CLI binaries
clean:
	rm -f *.out coverage.out $(BIN) elwi

## version: Display current version
version:
	@grep 'Version = ' version.go | sed 's/.*"\(.*\)".*/\1/'

## version-increment: Bump the patch version in version.go
version-increment:
	@VERSION=$$(grep 'Version = ' version.go | sed 's/.*"\(.*\)".*/\1/') && \
	MAJOR=$$(echo $$VERSION | cut -d. -f1) && \
	MINOR=$$(echo $$VERSION | cut -d. -f2) && \
	PATCH=$$(echo $$VERSION | cut -d. -f3) && \
	NEW_PATCH=$$((PATCH + 1)) && \
	NEW_VERSION="$$MAJOR.$$MINOR.$$NEW_PATCH" && \
	sed -i "s/Version = \"$$VERSION\"/Version = \"$$NEW_VERSION\"/" version.go && \
	echo "Bumped $$VERSION → $$NEW_VERSION"

## push: Bump patch version, run checks, commit, push, and tag
push: fmt tidy build test
	@VERSION=$$(grep 'Version = ' version.go | sed 's/.*"\(.*\)".*/\1/') && \
	MAJOR=$$(echo $$VERSION | cut -d. -f1) && \
	MINOR=$$(echo $$VERSION | cut -d. -f2) && \
	PATCH=$$(echo $$VERSION | cut -d. -f3) && \
	NEW_PATCH=$$((PATCH + 1)) && \
	NEW_VERSION="$$MAJOR.$$MINOR.$$NEW_PATCH" && \
	sed -i "s/Version = \"$$VERSION\"/Version = \"$$NEW_VERSION\"/" version.go && \
	echo "Bumped $$VERSION → $$NEW_VERSION" && \
	git add -A && \
	git commit -m "release: v$$NEW_VERSION" && \
	git push && \
	git tag v$$NEW_VERSION && \
	git push --tags && \
	echo "Released v$$NEW_VERSION"

## run: Build and run elw with ARGS (e.g. make run ARGS="What is 2+2?")
run: build-elw
	./$(BIN) $(ARGS)

## help: Show available targets
help:
	@echo "Usage: make <target>"
	@echo ""
	@grep '## ' Makefile | grep -v '@grep' | sed 's/## /  /' | column -t -s ':'
