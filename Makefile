.PHONY: native test test-consumer vet check fmt fmt-check lint lint-go clean

CARGO_TARGET_DIR ?= $(CURDIR)/internal/native/target
NATIVE_TARGET ?= $(CARGO_BUILD_TARGET)
NATIVE_LIB_DIR = $(CARGO_TARGET_DIR)/$(if $(NATIVE_TARGET),$(NATIVE_TARGET)/)release
CONSUMER_TARGET_DIR = $(CURDIR)/.consumer-native-target
CONSUMER_LIB_DIR = $(CONSUMER_TARGET_DIR)/$(if $(NATIVE_TARGET),$(NATIVE_TARGET)/)release
CONSUMER_BIN = $(CONSUMER_TARGET_DIR)/consumer$(if $(filter Windows_NT,$(OS)),.exe)

native:
	CARGO_TARGET_DIR="$(CARGO_TARGET_DIR)" cargo build --manifest-path internal/native/Cargo.toml --release --locked

test: native
	CARGO_TARGET_DIR="$(CARGO_TARGET_DIR)" cargo test --manifest-path internal/native/Cargo.toml --locked
	CGO_ENABLED=1 CGO_LDFLAGS="-L$(NATIVE_LIB_DIR) $(CGO_LDFLAGS)" go test -race ./...

test-consumer:
	@source_dir="$$(go -C testdata/consumer list -m -f '{{.Dir}}' github.com/dbast/go-rattler)"; \
	  CARGO_TARGET_DIR="$(CONSUMER_TARGET_DIR)" cargo build --manifest-path "$$source_dir/internal/native/Cargo.toml" --release --locked
	CGO_ENABLED=1 CGO_LDFLAGS="-L$(CONSUMER_LIB_DIR) $(CGO_LDFLAGS)" go -C testdata/consumer test ./...
	CGO_ENABLED=1 CGO_LDFLAGS="-L$(CONSUMER_LIB_DIR) $(CGO_LDFLAGS)" go -C testdata/consumer build -o "$(CONSUMER_BIN)" ./cmd/consumer
	$(CONSUMER_BIN)

vet: native
	CGO_ENABLED=1 CGO_LDFLAGS="-L$(NATIVE_LIB_DIR) $(CGO_LDFLAGS)" go vet ./...

check: test test-consumer vet fmt-check lint lint-go

fmt:
	rustfmt --edition 2024 internal/native/src/*.rs
	gofmt -w .

fmt-check:
	rustfmt --edition 2024 --check internal/native/src/*.rs
	gofmt -d .

lint:
	CARGO_TARGET_DIR="$(CARGO_TARGET_DIR)" cargo clippy --manifest-path internal/native/Cargo.toml --all-targets --locked -- -D warnings

lint-go:
	CGO_ENABLED=1 golangci-lint run ./...

clean:
	CARGO_TARGET_DIR="$(CARGO_TARGET_DIR)" cargo clean --manifest-path internal/native/Cargo.toml
