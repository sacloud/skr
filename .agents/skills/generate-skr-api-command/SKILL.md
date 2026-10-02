---
name: generate-skr-api-command
description: Add or evolve a skr command that wraps a public sacloud-sdk-go API, including Kong wiring, request handling, documentation, tests, and coverage checks.
---

# Generate a skr SDK API command

Use this skill when adding a `skr <service>-api` command backed by a public API in
`github.com/sacloud/sacloud-sdk-go`. Follow the repository's Go version and existing
Kong command structure; do not assume compatibility with the older `usacloud` CLI.

## Before implementation

1. Read `AGENTS.md`, `go.mod`, `main.go`, and the relevant tests and documentation.
2. Confirm the SDK version and inspect the public service package and API interfaces.
   Use the matching SDK package rather than manually building HTTP requests.
3. Use the SDK and `manual.sakura.ad.jp` as sources for service behavior. Do not invent
   resource semantics, request fields, prerequisites, or examples.
4. Review a referenced existing implementation when one is provided, but adapt it to
   skr's architecture rather than copying dependencies on usacloud packages.

## Command implementation

- Add the command to the root Kong CLI with an explicit command name when its spelling
  is not obvious from the Go field name.
- Model nested resources and operations as Kong command structs with `Run(*kong.Context)`
  methods. Keep API-client construction in a small shared helper.
- Initialize `saclient.Client` from `os.Environ()` and use the SDK service's client and
  operation constructors. Return SDK and serialization errors to Kong; do not silently
  substitute defaults or fall back to raw HTTP.
- Use the SDK request and response types. Preserve provider/type discriminators and
  request invariants required by the SDK operation. Set service-specific discriminators
  in the command layer when the SDK operation expects them and the command already
  determines the resource type.
- Make JSON input and output behavior explicit. If accepting request JSON, document the
  request type, inline JSON syntax, and any file syntax in command help and README.
- Expose only operations supported by the public SDK interface and appropriate for the
  requested scope.

## Verification

- Add tests that verify the root help exposes the command and that nested commands parse
  as intended. Test request decoding and error paths where applicable.
- For API behavior, use the corresponding `github.com/sacloud/sakumock/<service>` test
  server with dummy credentials and the SDK endpoint environment variable. Exercise the
  CLI through create/read/update/list/delete flows instead of relying only on fake API
  interfaces.
- Run `gofmt` on changed Go files and targeted package tests, then `go test ./...` when
  the change affects root CLI wiring.
- Update README usage examples for new user-visible commands.
