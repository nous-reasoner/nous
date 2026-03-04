VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS   = -s -w -X main.version=$(VERSION)
GOFLAGS   = -trimpath
BUILD_DIR = build
REL_DIR   = release

BINARIES  = nousd nous-cli
PKG_NOUSD = ./cmd/nousd
PKG_CLI   = ./cmd/nous-cli

# ── platform builds ─────────────────────────────────────────────

.PHONY: build-linux build-mac build-windows build-all release clean test

build-linux:
	@mkdir -p $(BUILD_DIR)/linux
	GOOS=linux GOARCH=amd64 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/linux/nousd    $(PKG_NOUSD)
	GOOS=linux GOARCH=amd64 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/linux/nous-cli $(PKG_CLI)
	@echo "✓ linux/amd64 → $(BUILD_DIR)/linux/"

build-mac:
	@mkdir -p $(BUILD_DIR)/darwin
	GOOS=darwin GOARCH=arm64 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/darwin/nousd    $(PKG_NOUSD)
	GOOS=darwin GOARCH=arm64 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/darwin/nous-cli $(PKG_CLI)
	@echo "✓ darwin/arm64 → $(BUILD_DIR)/darwin/"

build-windows:
	@mkdir -p $(BUILD_DIR)/windows
	GOOS=windows GOARCH=amd64 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/windows/nousd.exe    $(PKG_NOUSD)
	GOOS=windows GOARCH=amd64 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/windows/nous-cli.exe $(PKG_CLI)
	@echo "✓ windows/amd64 → $(BUILD_DIR)/windows/"

build-all: build-linux build-mac build-windows

# ── release packaging ───────────────────────────────────────────

release: build-all
	@mkdir -p $(REL_DIR)
	@cp README.md $(BUILD_DIR)/linux/README.txt   2>/dev/null || true
	@cp README.md $(BUILD_DIR)/darwin/README.txt  2>/dev/null || true
	@cp README.md $(BUILD_DIR)/windows/README.txt 2>/dev/null || true
	tar czf $(REL_DIR)/nous-linux-amd64.tar.gz  -C $(BUILD_DIR)/linux  nousd nous-cli README.txt
	tar czf $(REL_DIR)/nous-darwin-arm64.tar.gz  -C $(BUILD_DIR)/darwin nousd nous-cli README.txt
	cd $(BUILD_DIR)/windows && zip -q ../$(REL_DIR)/nous-windows-amd64.zip nousd.exe nous-cli.exe README.txt || \
		tar -a -cf ../../$(REL_DIR)/nous-windows-amd64.zip nousd.exe nous-cli.exe README.txt
	@echo ""
	@echo "Release archives:"
	@ls -lh $(REL_DIR)/

# ── helpers ─────────────────────────────────────────────────────

test:
	go test ./... -short -count=1 -timeout 180s

clean:
	rm -rf $(BUILD_DIR) $(REL_DIR)
