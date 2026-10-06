#====================
AUTHOR         ?= The sacloud/skr Authors
COPYRIGHT_YEAR ?= 2022-2026

BIN            ?= skr
GO_FILES       ?= $(shell find . -name '*.go')
GO_ENTRY_FILE  ?= .
DOCS_OUTPUT    ?= _site

include includes/go/common.mk
include includes/go/single.mk
#====================

default: $(DEFAULT_GOALS)
tools: dev-tools

.PHONY: generate-iaas-api
generate-iaas-api:
	@test -n "$(API_CONFIG)" || (echo "API_CONFIG is required" >&2; exit 2)
	@test -n "$(API_OUTPUT)" || (echo "API_OUTPUT is required" >&2; exit 2)
	$(GO) run ./cmd/apigen-iaas -config "$(API_CONFIG)" -out "$(API_OUTPUT)"

.PHONY: docs-site
docs-site:
	@set -eu; \
		tmp_dir=$$(mktemp -d); \
		trap 'rm -f "$$tmp_dir/skr"; rmdir "$$tmp_dir"' EXIT; \
		$(GO) build -o "$$tmp_dir/skr" .; \
		$(GO) run ./cmd/docsite -cli "$$tmp_dir/skr" -manual docs/manual -out "$(DOCS_OUTPUT)"
