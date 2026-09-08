PLUGIN_NAME := censorship
GO ?= go
VERSION ?=
NORMALIZED_VERSION := $(if $(strip $(VERSION)),$(patsubst v%,%,$(strip $(VERSION))),0.0.0-dev)

LIB_EXTENSION := .so
ifeq ($(GOOS),windows)
LIB_EXTENSION := .dll
else ifeq ($(GOOS),darwin)
LIB_EXTENSION := .dylib
endif

DIST_DIR = dist/$(GOOS)_$(GOARCH)
LIBRARY = $(DIST_DIR)/$(PLUGIN_NAME)$(LIB_EXTENSION)
ARCHIVE = dist/$(PLUGIN_NAME)_$(NORMALIZED_VERSION)_$(GOOS)_$(GOARCH).zip
CHECKSUM = $(ARCHIVE).sha256

.PHONY: test race vet integration build-platform build validate-version package-platform package clean

test:
	$(GO) test ./...

race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

integration:
	$(GO) test .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go
	$(GO) run ./.github/scripts/integration-runner.go

validate-version:
	@test -n "$(NORMALIZED_VERSION)" || { echo "VERSION must not normalize to an empty release version"; exit 2; }

build-platform:
	@test -n "$(GOOS)" && test -n "$(GOARCH)" || { echo "GOOS and GOARCH are required"; exit 2; }
	@mkdir -p "$(DIST_DIR)"
	CGO_ENABLED=1 $(if $(strip $(BUILD_CC)),CC="$(BUILD_CC)") GOOS="$(GOOS)" GOARCH="$(GOARCH)" $(GO) build -trimpath -buildmode=c-shared -ldflags='-s -w -X main.pluginVersion=$(NORMALIZED_VERSION)' -o "$(LIBRARY)" .

build:
	@echo "build is host-specific; use build-platform for a selected GOOS/GOARCH target."
	$(MAKE) build-platform GOOS="$(shell $(GO) env GOHOSTOS)" GOARCH="$(shell $(GO) env GOHOSTARCH)" VERSION="$(VERSION)" BUILD_CC="$(BUILD_CC)"

package-platform: validate-version
	$(MAKE) build-platform GOOS="$(GOOS)" GOARCH="$(GOARCH)" VERSION="$(VERSION)" BUILD_CC="$(BUILD_CC)"
	$(GO) run ./.github/scripts/package-release.go -version "$(NORMALIZED_VERSION)" -library "$(LIBRARY)" -archive "$(ARCHIVE)" -checksum "$(CHECKSUM)"

package: validate-version
ifeq ($(strip $(GOOS)),)
ifeq ($(strip $(GOARCH)),)
package:
	$(GO) run ./.github/scripts/package-release.go -dist dist -out dist -version "$(NORMALIZED_VERSION)"
else
package:
	$(MAKE) package-platform GOOS="$(GOOS)" GOARCH="$(GOARCH)" VERSION="$(VERSION)" BUILD_CC="$(BUILD_CC)"
endif
else
package:
	$(MAKE) package-platform GOOS="$(GOOS)" GOARCH="$(GOARCH)" VERSION="$(VERSION)" BUILD_CC="$(BUILD_CC)"
endif

clean:
	rm -rf dist
