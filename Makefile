PLUGIN_NAME := censorship
GO ?= go
RAW_VERSION := $(value VERSION)
PACKAGER_VERSION := $(if $(filter undefined,$(origin VERSION)),0.0.0-dev,$(RAW_VERSION))
unexport VERSION
NORMALIZED_VERSION = $(patsubst v%,%,$(PACKAGER_VERSION))
export PACKAGER_VERSION NORMALIZED_VERSION

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
	@version="$$PACKAGER_VERSION"; case "$$version" in ""|[!A-Za-z0-9]*|*[!A-Za-z0-9._+-]*) echo "VERSION must normalize to a safe non-empty release version"; exit 2;; esac; version="$${version#v}"; case "$$version" in ""|[!A-Za-z0-9]*|*[!A-Za-z0-9._+-]*) echo "VERSION must normalize to a safe non-empty release version"; exit 2;; esac

build-platform: validate-version
	@test -n "$(GOOS)" && test -n "$(GOARCH)" || { echo "GOOS and GOARCH are required"; exit 2; }
	@mkdir -p "$(DIST_DIR)"
	CGO_ENABLED=1 $(if $(strip $(BUILD_CC)),CC="$(BUILD_CC)") GOOS="$(GOOS)" GOARCH="$(GOARCH)" $(GO) build -trimpath -buildmode=c-shared -ldflags="-s -w -X main.pluginVersion=$$NORMALIZED_VERSION" -o "$(LIBRARY)" .

build:
	@echo "build is host-specific; use build-platform for a selected GOOS/GOARCH target."
	$(MAKE) build-platform GOOS="$(shell $(GO) env GOHOSTOS)" GOARCH="$(shell $(GO) env GOHOSTARCH)" VERSION="$${PACKAGER_VERSION}" BUILD_CC="$(BUILD_CC)"

package-platform: validate-version
	$(MAKE) build-platform GOOS="$(GOOS)" GOARCH="$(GOARCH)" VERSION="$${PACKAGER_VERSION}" BUILD_CC="$(BUILD_CC)"
	$(GO) run ./.github/scripts/package-release.go -version "$$PACKAGER_VERSION" -library "$(LIBRARY)" -archive "$(ARCHIVE)" -checksum "$(CHECKSUM)"

package: validate-version
ifeq ($(strip $(GOOS)),)
ifeq ($(strip $(GOARCH)),)
package:
	$(GO) run ./.github/scripts/package-release.go -dist dist -out dist -version "$$PACKAGER_VERSION"
else
package:
	$(MAKE) package-platform GOOS="$(GOOS)" GOARCH="$(GOARCH)" VERSION="$${PACKAGER_VERSION}" BUILD_CC="$(BUILD_CC)"
endif
else
package:
	$(MAKE) package-platform GOOS="$(GOOS)" GOARCH="$(GOARCH)" VERSION="$${PACKAGER_VERSION}" BUILD_CC="$(BUILD_CC)"
endif

clean:
	rm -rf dist
