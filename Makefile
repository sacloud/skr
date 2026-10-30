#====================
AUTHOR         ?= The sacloud/skr Authors
COPYRIGHT_YEAR ?= 2022-2026

BIN            ?= skr
GO_FILES       ?= $(shell find . -name '*.go')
GO_ENTRY_FILE  ?= ./cmd/skr
GO_INSTALL_PACKAGE ?= ./cmd/skr
COPYRIGHT_FILES ?= $$(find . -name "*.go" -print | grep -v "/vendor/" | grep -v "_generated\.go$$")
GO_LICENSES_PACKAGE ?= ./...
DOCS_OUTPUT    ?= _site

include includes/go/common.mk
include includes/go/single.mk
#====================

default: $(DEFAULT_GOALS)
tools: dev-tools

.PHONY: lint-adr
lint-adr:
	@awk 'FNR == 1 { \
		file = FILENAME; \
		sub(/^.*\//, "", file); \
		number = substr(file, 1, 4); \
		files[number] = files[number] " " file; \
		counts[number]++; \
	} END { \
		for (number in counts) { \
			if (counts[number] > 1) { \
				printf "duplicate ADR number %s:%s\n", number, files[number] > "/dev/stderr"; \
				failed = 1; \
			} \
		} \
		exit failed; \
	}' docs/adr/[0-9][0-9][0-9][0-9]-*.md

lint-text: lint-adr

.PHONY: e2e-coverage
e2e-coverage:
	@echo "reporting E2E command coverage..."
	@$(GO) test ./test/e2e/command-coverage/ -v -run 'TestBaselineCoversAllLeafCommands|TestReportEvidenceCoverage'

.PHONY: generate-api
generate-api:
	@test -n "$(API_CONFIG)" || (echo "API_CONFIG is required" >&2; exit 2)
	@test -n "$(API_OUTPUT)" || (echo "API_OUTPUT is required" >&2; exit 2)
	$(GO) run ./cmd/apigen-api -config "$(API_CONFIG)" -out "$(API_OUTPUT)"

.PHONY: generate-iaas-api
generate-iaas-api:
	@test -n "$(API_CONFIG)" || (echo "API_CONFIG is required" >&2; exit 2)
	@test -n "$(API_OUTPUT)" || (echo "API_OUTPUT is required" >&2; exit 2)
	$(GO) run ./cmd/apigen-iaas -config "$(API_CONFIG)" -out "$(API_OUTPUT)"

.PHONY: generate-wiring
generate-wiring:
	$(GO) run ./cmd/apigen-wiring -mode iaas -out internal/cli/iaas_wiring_generated.go \
		$(patsubst %,api/commands/%,$(filter iaas-%,$(notdir $(wildcard api/commands/*.json))))
	$(GO) run ./cmd/apigen-wiring -mode ogen -out internal/cli/eventbus_wiring_generated.go \
		$(patsubst %,api/commands/%,$(filter eventbus-%,$(notdir $(wildcard api/commands/*.json))))
	$(GO) run ./cmd/apigen-wiring -mode ogen -out internal/cli/iam_wiring_generated.go \
		$(patsubst %,api/commands/%,$(filter iam-%,$(notdir $(wildcard api/commands/*.json))))
	$(GO) run ./cmd/apigen-wiring -mode ogen -out internal/cli/kms_wiring_generated.go \
		$(patsubst %,api/commands/%,$(filter kms-%,$(notdir $(wildcard api/commands/*.json))))
	$(GO) run ./cmd/apigen-wiring -mode ogen -out internal/cli/simplemq_wiring_generated.go \
		$(patsubst %,api/commands/%,$(filter simplemq-%,$(notdir $(wildcard api/commands/*.json))))
	$(GO) run ./cmd/apigen-wiring -mode ogen -out internal/cli/simple_notification_wiring_generated.go \
		$(patsubst %,api/commands/%,$(filter simple-notification-%,$(notdir $(wildcard api/commands/*.json))))
	$(GO) run ./cmd/apigen-wiring -mode ogen -out internal/cli/apprun_wiring_generated.go \
		$(patsubst %,api/commands/%,$(filter apprun-%,$(notdir $(wildcard api/commands/*.json))))
	$(GO) run ./cmd/apigen-wiring -mode ogen -out internal/cli/secretmanager_wiring_generated.go \
		$(patsubst %,api/commands/%,$(filter secretmanager-%,$(notdir $(wildcard api/commands/*.json))))

.PHONY: docs-site
docs-site:
	@set -eu; \
		tmp_dir=$$(mktemp -d); \
		trap 'rm -f "$$tmp_dir/skr"; rmdir "$$tmp_dir"' EXIT; \
		$(GO) build -o "$$tmp_dir/skr" ./cmd/skr; \
		$(GO) run ./cmd/docsite -cli "$$tmp_dir/skr" -manual docs/manual -out "$(DOCS_OUTPUT)"
