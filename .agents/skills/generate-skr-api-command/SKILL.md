---
name: generate-skr-api-command
description: Add or evolve a skr command backed by a public sacloud-sdk-go API, including command design, self-documenting help, sakumock tests, and build verification.
---

# Generate a skr SDK API command

Use this skill when adding a `skr <service>-api` command backed by
`github.com/sacloud/sacloud-sdk-go`. Build for skr's current architecture and Go version.
Do not assume compatibility with the older `usacloud` CLI or copy its internal packages.

## Guiding principles

- Treat CLI help as the primary user guide. A user should be able to understand the
  service workflow, prepare valid input, run the command, and interpret its output from
  `--help` without first reading source code.
- Base service behavior, field meanings, prerequisites, units, and examples only on the
  target SDK's documentation and `manual.sakura.ad.jp`. Do not fill gaps with guesses,
  prior knowledge, or examples from unrelated services.
- Wrap public SDK interfaces and models. Do not recreate SDK behavior with raw HTTP or
  copy SDK-internal implementation code.
- Keep changes focused, preserve existing behavior, and document all new user-facing
  commands.

## Workflow

### 1. Establish the supported API and service workflow

Read `AGENTS.md`, `go.mod`, `main.go`, the target SDK package, and relevant tests and
documentation. Confirm the SDK version used by this repository and identify:

- Public service operation interfaces and their methods.
- Resource types, request and response models, union/discriminator fields, and
  optional/null semantics.
- Required setup order and how resources refer to each other.
- Authentication, endpoint configuration, units, supported values, and any service
  limitations that should be visible to users.

Use the SDK and official manual as evidence for service behavior. If a critical field,
discriminator, resource relationship, or prerequisite is unclear in these sources, do
not invent an answer; narrow the feature or stop and report what needs clarification.
When a prior implementation is supplied, use it as a reference, adapting its behavior
to skr rather than importing its framework or dependencies.

### 2. Design the command tree and its help

Plan the command path, resource names, operation names, request shape, and output before
implementing handlers. Follow the existing Kong patterns in `main.go` and the adjacent
command code:

- Register the command on the root CLI with an explicit `name` when the Go field name
  does not produce the desired spelling.
- Represent nested resources and operations with Kong command structs and
  `Run(*kong.Context) error` handlers.
- Use resource-specific command types when a shared generic description would hide
  resource-specific fields, constraints, or examples.
- Keep SDK client construction in a small helper and initialize `saclient.Client` from
  `os.Environ()`. Use the SDK service client and operation constructors.
- Return SDK, input, file, and output errors to Kong; do not silently ignore errors or
  substitute success-shaped defaults.

Write help for the actual rendered command levels: service, resource, operation, and
flags. Explain, where applicable:

- What each resource represents and how it relates to other resources.
- Which resources to create first and which IDs to use in later commands.
- Required and optional fields, valid values, units, and mutually exclusive choices.
- Whether updates are partial, how omitted fields behave, and how to clear nullable
  fields.
- Authentication/configuration expectations and output format.
- Secret file/stdin handling without encouraging secrets in command-line arguments.
- A copyable command example with clearly marked values the user must replace.

Keep examples accurate to the SDK/manual. Mark event sources, IDs, credentials, and
other environment-specific values as placeholders when they cannot be universally
valid. Avoid putting secrets in inline command examples.

### 3. Implement SDK request and response handling

- Use the public SDK request and response types. Preserve required provider classes,
  sum-type selections, and other operation invariants. If the command identifies the
  resource type, set its service-specific discriminator in the command layer when
  required by the SDK.
- Make request input syntax explicit in flag help. If accepting JSON, state its SDK
  request shape, whether it can be passed inline or as `@path.json`, and any
  command-supplied fields. Keep complex examples in files in the documentation to avoid
  shell-quoting ambiguity.
- Use SDK JSON decoding/encoding behavior for SDK models, especially models with
  optional values or sum types. Do not assume ordinary Go field JSON behavior is
  equivalent.
- Document and test the output shape. Preserve useful scalar IDs or resource data in
  output; do not return a success-shaped result when the SDK reports an error.
- Expose only operations supported by the public service API and justified by the task.

### 4. Add tests using sakumock

Use `github.com/sacloud/sakumock/<service>` for API behavior tests:

- Start the service's test server and close it with test cleanup.
- Configure dummy credentials and the SDK endpoint environment variable; use an
  isolated profile directory if profile discovery could affect the test.
- Invoke the real CLI command path against sakumock. Verify the outgoing request's
  resource class/settings and returned result through create, list, read, update, and
  delete flows for each implemented resource.
- Verify secret-setting operations with sakumock's inspection API when available.
- Add focused unit tests for request decoding, file/stdin handling, validation, and
  error paths. Add help tests that assert key workflow, input, and example details in
  rendered help, not merely that command names exist.

Fake API implementations may help isolate pure command logic, but they are not a
replacement for sakumock coverage of the SDK request and response path.

### 5. Document and verify the full command

- Update README usage instructions with the authentication/configuration expectations,
  setup order, representative commands, request-file usage, output behavior, and
  relevant cautions.
- Run `gofmt` on changed Go files, targeted tests, and `go test ./...` when root command
  registration or shared CLI behavior changes.
- Run `make lint-go` and relevant documentation checks when available.
- Run `make build` and smoke-test the built command's help.
- The shared Go Makefile defaults `GO_ENTRY_FILE` to `main.go`, which builds only that
  file. If the main package uses multiple source files, set `GO_ENTRY_FILE ?= .` in
  this repository's `Makefile` so `make build` compiles the package, not only `main.go`.
- Review `git diff --check` and verify no unrelated files or generated artifacts were
  introduced.
