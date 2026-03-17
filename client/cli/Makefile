VERSION := 0.3.0
LDFLAGS := -s -w -X main.version=$(VERSION)
PLATFORMS := linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 windows-amd64

.PHONY: build build-session-config test lint bench coverage install clean build-all package package-release $(addprefix build-,$(PLATFORMS)) $(addprefix build-session-config-,$(PLATFORMS))

build:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o bin/veilkey-cli .

build-session-config:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o bin/veilkey-session-config ./cmd/veilkey-session-config

test:
	go test -v -race ./...

lint:
	golangci-lint run ./...

bench:
	go test -bench=. -benchmem ./...

coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	@echo ""
	@echo "HTML report: go tool cover -html=coverage.out"

install: build build-session-config
	cp bin/veilkey-cli /usr/local/bin/veilkey-cli
	cp bin/veilkey-session-config /usr/local/bin/veilkey-session-config

clean:
	rm -rf bin/ dist/ coverage.out

build-linux-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o bin/veilkey-cli-linux-amd64 .

build-session-config-linux-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o bin/veilkey-session-config-linux-amd64 ./cmd/veilkey-session-config

build-linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o bin/veilkey-cli-linux-arm64 .

build-session-config-linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o bin/veilkey-session-config-linux-arm64 ./cmd/veilkey-session-config

build-darwin-amd64:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o bin/veilkey-cli-darwin-amd64 .

build-session-config-darwin-amd64:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o bin/veilkey-session-config-darwin-amd64 ./cmd/veilkey-session-config

build-darwin-arm64:
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o bin/veilkey-cli-darwin-arm64 .

build-session-config-darwin-arm64:
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o bin/veilkey-session-config-darwin-arm64 ./cmd/veilkey-session-config

build-windows-amd64:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o bin/veilkey-cli-windows-amd64.exe .

build-session-config-windows-amd64:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o bin/veilkey-session-config-windows-amd64.exe ./cmd/veilkey-session-config

build-all: $(addprefix build-,$(PLATFORMS)) $(addprefix build-session-config-,$(PLATFORMS))

package: build-all
	@mkdir -p dist/manual
	@for p in linux-amd64 linux-arm64 darwin-amd64 darwin-arm64; do \
		echo "Packaging veilkey-cli-$$p..."; \
		tmpdir=$$(mktemp -d); \
		cp "bin/veilkey-cli-$$p" "$$tmpdir/veilkey-cli"; \
		cp "bin/veilkey-session-config-$$p" "$$tmpdir/veilkey-session-config"; \
		cp scripts/vk "$$tmpdir/vk"; \
		cp deploy/host/veil "$$tmpdir/veil"; \
		tar -czf "dist/manual/veilkey-cli-$(VERSION)-$$p.tar.gz" -C "$$tmpdir" veilkey-cli veilkey-session-config vk veil; \
		rm -rf "$$tmpdir"; \
	done
	@if command -v zip >/dev/null 2>&1; then \
		echo "Packaging veilkey-cli-windows-amd64..."; \
		tmpdir=$$(mktemp -d); \
		cp bin/veilkey-cli-windows-amd64.exe "$$tmpdir/veilkey-cli.exe"; \
		cp bin/veilkey-session-config-windows-amd64.exe "$$tmpdir/veilkey-session-config.exe"; \
		( cd "$$tmpdir" && zip -q "$(CURDIR)/dist/manual/veilkey-cli-$(VERSION)-windows-amd64.zip" veilkey-cli.exe veilkey-session-config.exe ); \
		rm -rf "$$tmpdir"; \
	else \
		echo "Skipping Windows package: zip not found"; \
	fi
	@echo "Done: dist/manual/"
	@ls -lh dist/manual/

package-release:
	goreleaser release --clean --snapshot --skip=publish
