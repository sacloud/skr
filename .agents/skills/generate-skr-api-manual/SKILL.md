---
name: generate-skr-api-manual
description: Create user-focused Markdown tutorials for implemented skr API commands, grounded in official Sakura Cloud documentation and verified command behavior.
---

# Generate a skr API tutorial

Use this skill to write a user-facing tutorial for an implemented
`skr <service>-api` command. The tutorial describes the service workflow as well as
the CLI syntax; it is not a command reference copied from `--help`.

Generated tutorials are Markdown files under `docs/tutorials/`, named
`<service>-api.md`. Do not generate reStructuredText.

## Principles and source policy

- Base service behavior, use cases, resource meanings and relationships, prerequisites,
  constraints, and examples only on the official service manual under
  `https://manual.sakura.ad.jp/cloud/` and documentation in the exact
  `sacloud-sdk-go` version used by this repository.
- Do not infer product behavior from prior knowledge, search summaries, third-party
  articles, or examples for another service. Record a source for each service-specific
  factual claim. If a required fact has no authoritative source, omit it or stop and
  explain what is missing.
- Use the current skr command implementation and its rendered `--help` as the authority
  for command paths, flags, JSON request shapes, output behavior, and file/stdin syntax.
  Do not copy usacloud commands or promise usacloud compatibility.
- Clearly distinguish facts supported by documentation from behavior verified by tests.
  Never present an unverified scenario as a successful live end-to-end result.
- Keep credentials and account-specific data out of the tutorial. Use placeholders for
  IDs, names, event sources, zones, and other environment-specific values. Do not include
  real profile names, API tokens, secrets, account IDs, production output, or logs.

## Inputs and deliverables

Before drafting, confirm:

- The target command exists and its `--help` describes its current syntax.
- The repository SDK version and the target service's SDK documentation.
- The official manual pages that substantiate the service model and steps.
- The resources, dependencies, credentials, permissions, and observations required by a
  useful end-to-end scenario.
- The exact resources a live test would create, change, and delete, and how to clean
  them up safely.

Create the tutorial at `docs/tutorials/<service>-api.md`. Use
`assets/tutorial.md.tmpl` as a structure, not as text to leave filled with placeholders.
Keep a temporary evidence table outside the repository; do not commit research notes.
Include the source URL next to each fact in the evidence table, and retain only claims
that can be traced to the official manual or the pinned SDK documentation.

## Writing workflow

1. Read `AGENTS.md`, `go.mod`, the command implementation, relevant tests, and the
   target SDK service documentation.
2. Read the service manual from `manual.sakura.ad.jp`. Establish the problem the service
   solves, the concepts and resource relationships, valid prerequisites, constraints,
   and the intended result of the tutorial.
3. Check the rendered root, resource, and operation help. Confirm the tutorial's JSON
   examples against the implemented request decoder and SDK models. If help or code
   disagrees with the documentation, do not silently choose one; resolve the discrepancy
   before publishing instructions.
4. Design a small, reproducible scenario that demonstrates useful service behavior,
   not just successful `create`, `read`, or `list` requests. Identify how the user can
   observe the actual result, including any required destination or dependent service.
5. Separate setup, authentication, and pre-existing resources from resources created by
   the tutorial. State all user-supplied values and permissions needed before commands
   that mutate resources.
6. Draft Markdown using `references/markdown-writing-guideline.md` and the template.
   Explain the resource creation order, provide copyable commands or JSON files, verify
   the observable result, and give cleanup commands in reverse dependency order.
7. Review every command against the CLI's real `--help`; review every service fact
   against the evidence table. Mark non-universal values as placeholders and never
   invent event types, enum values, source identifiers, or successful outputs.
8. Run the Markdown link checker and its tests. Preview the document and verify local
   links, headings, fenced code blocks, and command rendering.
9. Report the generated tutorial path and whether its end-to-end behavior was verified
   with sakumock, a live service, or documentation alone. State any remaining manual
   verification or dependency clearly.

## Live service safety

A live tutorial test may create billable or production resources, change existing
resources, send notifications or messages, trigger automation, or delete data.
Never perform those actions based only on the request to write a tutorial.

Before any live mutation:

1. Show the user the target profile/zone or project, dependencies, exact resources and
   actions, expected side effects, and cleanup plan.
2. Obtain explicit approval for that specific live test.
3. Check the active profile/configuration and use only the approved account and scope.
4. Avoid reusing or deleting pre-existing resources. Use uniquely named test resources.
5. Keep secret values in a local protected file or supported secure input; do not ask the
   user to paste credentials into the conversation or place them in shell history.
6. Confirm the observed service behavior, then clean up only the resources created for
   the test and verify their removal.

If approval, required dependencies, safe cleanup, or a way to observe the actual
behavior is unavailable, do not claim live end-to-end verification. Use sakumock for
request/response behavior where supported and state the limit. If the tutorial's main
claim cannot be explained or substantiated without the missing live test, stop and
report the blocker rather than substituting a CRUD-only test.

## Required tutorial sections

Every tutorial should cover, when applicable:

1. **Purpose** — when and why to use the service, supported by official sources.
2. **Prerequisites** — skr version/command, authentication configuration, permissions,
   pre-existing resources, and any dependent service.
3. **Workflow and resources** — what will be created and in which order.
4. **Inspect command help** — root/resource/operation help relevant to the steps.
5. **Create/configure** — valid JSON files or commands and explanations for required
   fields, units, choices, and placeholders.
6. **Verify behavior** — an observable service result beyond resource CRUD, or an
   explicit statement of which part was not live-verified and why.
7. **Clean up** — delete only tutorial-created resources in reverse dependency order
   and verify removal.
8. **References** — links to official manual pages and relevant CLI help commands.

Do not force irrelevant sections or facts into a tutorial. Prefer a concise, complete
scenario over a long catalog of API fields.

## Validation

Run the checker tests:

```sh
python3 -m unittest discover -s .agents/skills/generate-skr-api-manual/scripts -p 'test_*.py'
```

Check links in generated tutorials:

```sh
python3 ./.agents/skills/generate-skr-api-manual/scripts/check_tutorial_links.py
```

The checker can also receive explicit Markdown files and supports `--timeout`, `--retries`,
and `--jobs`. A failed external link must be rechecked against the official source and
updated if it moved; do not remove a correct citation merely to make the check pass.

## References

- [Markdown writing guidelines](references/markdown-writing-guideline.md)
- [Tutorial template](assets/tutorial.md.tmpl)
- [Markdown external-link checker](scripts/check_tutorial_links.py)
