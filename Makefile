#====================
AUTHOR         ?= The sacloud/skr Authors
COPYRIGHT_YEAR ?= 2022-2026

BIN            ?= skr
GO_FILES       ?= $(shell find . -name '*.go')
GO_ENTRY_FILE  ?= .

include includes/go/common.mk
include includes/go/single.mk
#====================

default: $(DEFAULT_GOALS)
tools: dev-tools
