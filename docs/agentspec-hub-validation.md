# AgentSpec Hub CLI validation

Scope: extend the existing `agentspec-list` / `agentspec-get` commands to mirror
Skill discovery and fixed-version downloading. Keep native AgentSpec JSON files,
`~/.agentspecs/<resource-name>`, exact-name list filtering, and ordinary version-label
resolution. Share the Profile `source` setting; add no standalone Hub commands.

## Automated regression

Run tests with an isolated `HOME` **and** `USERPROFILE`, so no test can overwrite a
real user's profiles on Windows. Do not run configuration fixture tests against
the user's actual home directory.

| Area | Scenarios |
| --- | --- |
| Source | `namespace`, `hub`, `all`, absent defaults to all; explicit flags override; explicit entry ID overrides Profile |
| Validation | `--hub` and `--no-hub` conflict in both orders; entry ID conflicts with `--no-hub`; invalid source and paging bounds fail |
| Pagination | Each selected source gets its own page/size/count; later exact-name pages are searched; equal names retain distinct IDs |
| Wire model | Parse nested `hub` metadata, never infer identity from labels/latest; use Hub timestamp/download count; preserve ordinary nullable governance fields |
| Identity | Keep authenticated session and business Namespace, including after interactive `ns`; reject manual internal Namespace selection |
| Availability | Empty/disabled Hub; permission and HTTP errors fail instead of hiding partial failures; revoked/changed share never falls back to source latest |
| Selection | Ordinary private online candidates remain; draft-only candidates are listed but not offered for Client download; duplicate source choices, cancellation, invalid choice, scripts |
| Version | Omitted version uses fixed share; matching explicit version works; mismatched version and non-latest Hub label fail; ordinary labels retain precedence |
| Files | Request uses Hub ID, local folder uses resource name; manifest/resource paths and binary base64 decoding retained; errors do not report success |
| Output | Independent JSON page shapes, human-readable identity and governance; terminal colors; `NO_COLOR`; JSON/direct redirected output free of ANSI |
| Compatibility | Existing list/get/service entry points; ordinary Client-only `--no-hub`; multi-name batch progress, summary and partial failure; native upload/review/release/describe |
| Interactive | Same handlers, source reload per invocation, active Namespace switching, help and flag completion |

New tests are in `internal/agentspec/agentspec_source_test.go`,
`cmd/agentspec_hub_test.go`, and `internal/terminal/agentspec_hub_test.go`.
Existing Skill tests must also pass, as source and rendering helpers are reused.

## Deployed integration checklist

These checks require a running matching Nacos server and appropriate accounts.
Mock HTTP tests do not prove deployed integration.

1. Configure one Profile with `source=all`; verify both Skill and AgentSpec obey it.
2. Create published public AgentSpecs with the same name in two source Namespaces;
   share explicit different versions. Add private published and draft-only resources
   in the active business Namespace. Compare authorized Console and CLI listings.
3. Check `agentspec-list`, `--hub`, `--no-hub`, name filtering, JSON and multiple
   pages. Confirm Hub IDs, source names and independent counts.
4. Download by name and select each candidate; repeat by explicit Hub ID and with
   an omitted/matching version. Verify manifest and all text/binary resources.
5. Check a mismatched version, non-latest label, unknown ID, wrong display name,
   denied Hub read, and non-interactive ambiguity. No fallback or false success.
6. Revoke/adjust the share between discovery and get. Confirm fixed-version failure;
   re-list and select again. Confirm business Namespace and existing sync state remain.
7. Disable Hub and verify ordinary Namespace listing/downloading still works with
   `--no-hub`. Verify all mode sees an empty Hub page on the matching server.
8. Repeat in `interactive`, switch the business Namespace, and edit Profile `source`
   while it is open. Verify retained authentication and renewed source preference.
9. Verify source upload, review, release, describe, label-based get and batch get
   still behave normally. AgentSpec get downloads files; it does not import a draft.

No deployed integration run is implied by this checklist. Record executed checks,
server version and permissions separately; do not describe unexecuted checks as passed.

## Local verification, 2026-11-02

- Go 1.27.3: build and `go vet ./...` passed.
- Complete AgentSpec/config/client package tests: 36 top-level tests passed.
- Focused cross-package AgentSpec/Skill/Profile/terminal regression: 48 top-level
  tests passed. These selections overlap; their counts must not be added together.
- Full Windows suite: 168 top-level tests passed, 26 failed. The exact same 26 test
  names also fail in an isolated copy of the pre-change commit `7ce401a`, covering
  existing symlink permissions, synchronization paths and path-completion assumptions.
  The full suite is not passing.
- The previous temporary Go 1.21.13 installation lacks `pkg/include/textflag.h`;
  verification using that toolchain could not compile. This is not a passed
  minimum-version check, and the installation was not modified.
- Existing local Nacos at `127.0.0.1:8848`: current-Namespace list (2 entries), Hub
  list (3 entries), exact-name filter, downloads of all 3 Hub entries, matching
  explicit version, and wrong-version rejection verified. One isolated successful
  get increased the selected Hub entry's counter by one. Actual interactive Hub
  listing displayed the same three entries with terminal colors.
- The existing default Profile has `source=namespace`; the unflagged command
  correctly kept that preference, and `--hub` overrode it. Its file hash did not
  change during validation. Combined pages were verified with HTTP fixtures.
- Those three deployed test entries return empty `content` and empty `resource`;
  downloads therefore preserve an empty manifest. Full text/binary package layout
  was verified using HTTP fixtures, not those empty deployed entries. No source
  content is synthesized and no new requirement for JSON manifest content is imposed.
- Different-account permissions, real same-name shares across source Namespaces,
  concurrent share adjustment/revocation, and the Hub switch were not exercised
  against this deployment. These remain on the integration checklist above.

Detailed local logs and the pre-change source archive are under the workspace's
`_git-maintenance/20261102-agentspec-cli/`, outside this repository.
