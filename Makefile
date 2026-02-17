# n-compasstv Makefile
# Cross-compiles for Raspberry Pi 5 (arm64) and builds .deb packages.

APP_NAME    := n-compasstv
VERSION     := $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.1.0-dev")
BUILD_TIME  := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS     := -s -w -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)

GO          := go
GOFLAGS     := -trimpath
CGO_ENABLED := 1

# Target architecture for RPi5
GOOS        := linux
GOARCH      := arm64

BUILD_DIR   := build
BINARY      := $(BUILD_DIR)/$(APP_NAME)

.PHONY: all build build-local clean test lint deb install docker

# ---------- Build Targets ----------

all: clean build

# Cross-compile for RPi5 (arm64 Linux)
build:
	@echo "==> Building $(APP_NAME) $(VERSION) for $(GOOS)/$(GOARCH)"
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/player

# Build for the current host OS/arch (development)
build-local:
	@echo "==> Building $(APP_NAME) $(VERSION) (local)"
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) \
		$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/player

# ---------- Testing ----------

test:
	$(GO) test -v -race ./...

lint:
	golangci-lint run ./...

# ---------- Debian Package ----------

deb: build
	@echo "==> Building .deb package"
	@bash scripts/package.sh $(VERSION) $(BUILD_DIR)

# ---------- Installation (for local RPi5 development) ----------

install: build-local
	@echo "==> Installing $(APP_NAME)"
	sudo cp $(BINARY) /usr/local/bin/$(APP_NAME)
	sudo chmod +x /usr/local/bin/$(APP_NAME)
	sudo mkdir -p /playlist /etc/n-compasstv /var/log/n-compasstv
	sudo cp deploy/n-compasstv.service /etc/systemd/system/
	sudo systemctl daemon-reload
	sudo systemctl enable $(APP_NAME)
	@echo "==> Installed. Run: sudo systemctl start $(APP_NAME)"

# ---------- Docker ----------

docker:
	@echo "==> Building Docker image"
	docker build -t $(APP_NAME):$(VERSION) .

# ---------- Clean ----------

clean:
	@echo "==> Cleaning build artifacts"
	rm -rf $(BUILD_DIR)
