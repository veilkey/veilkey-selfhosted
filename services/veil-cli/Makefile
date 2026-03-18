VERSION := 0.1.0
CARGO   ?= cargo
PLATFORMS := linux-amd64 linux-arm64 darwin-amd64 darwin-arm64

.PHONY: build release test lint install clean \
        $(addprefix build-,$(PLATFORMS)) package

build:
	$(CARGO) build

release:
	$(CARGO) build --release

test:
	$(CARGO) test

lint:
	$(CARGO) clippy -- -D warnings

install: release
	cp target/release/veil    /usr/local/bin/veil
	cp target/release/veilkey /usr/local/bin/veilkey

clean:
	$(CARGO) clean

# Cross-compilation targets.
# For Linux targets from macOS, set CARGO=cross (needs: cargo install cross).
build-linux-amd64:
	$(CARGO) build --release --target x86_64-unknown-linux-gnu

build-linux-arm64:
	$(CARGO) build --release --target aarch64-unknown-linux-gnu

build-darwin-amd64:
	$(CARGO) build --release --target x86_64-apple-darwin

build-darwin-arm64:
	$(CARGO) build --release --target aarch64-apple-darwin

# Package binaries into dist/manual/<platform>.tar.gz
package: $(addprefix build-,$(PLATFORMS))
	@mkdir -p dist/manual
	@for entry in \
	    "linux-amd64:x86_64-unknown-linux-gnu" \
	    "linux-arm64:aarch64-unknown-linux-gnu" \
	    "darwin-amd64:x86_64-apple-darwin" \
	    "darwin-arm64:aarch64-apple-darwin"; do \
	    plat=$${entry%%:*}; triple=$${entry##*:}; \
	    echo "Packaging veil-cli-rs-$$plat..."; \
	    tmpdir=$$(mktemp -d); \
	    cp target/$$triple/release/veil    $$tmpdir/veil; \
	    cp target/$$triple/release/veilkey $$tmpdir/veilkey; \
	    tar -czf dist/manual/veil-cli-rs-$(VERSION)-$$plat.tar.gz -C $$tmpdir \
	        veil veilkey; \
	    rm -rf $$tmpdir; \
	done
	@echo "Done: dist/manual/"
	@ls -lh dist/manual/
