# Public Readiness Checklist

**Version:** 0.3
**Status:** Published. All audit items resolved and verified.
**Purpose:** Track the work completed before Atlas was published as an
open "early design/prototype" Ceph operations project.

Positioning: early-stage design and prototype work for a Ceph operations
platform. Not production-ready, not announced as ready for users.

Note on scope: this checklist is itself public. It records the *categories*
of internal references that were found and removed, without reproducing the
internal names themselves.

---

# 1. Audit Results (2026-08-14)

## Already clean at audit time

- No secrets, credentials, `.env` files, tokens, or private keys in the tree.
- No internal IP addresses, real hostnames, or CFT references found.
- Fixtures are synthetic: `example.invalid` hosts, all-zero-pattern FSIDs,
  generic device serials (`nvme-serial-a`).
- `LICENSE` (Apache 2.0) already existed.
- The working copy had no `.git` directory, so the public repository created
  from this tree started with clean history by construction.

## Fixes applied (2026-08-14)

| # | Item | Resolution |
|---|------|------------|
| F1 | Go module path used an internal Git host | Rewritten to `github.com/tonymontoya/ceph-atlas` in `go.mod` and all imports; `dev-plans/repository_layout.md` open question resolved |
| F2 | Internal organization name used as a domain-model example | Replaced with generic examples (`Example Organization`, `Acme Corporation`) |
| F3 | Real-environment description | `dev-plans/environment_context.md` rewritten as "example operating environment" (dropped environment-identifying specifics); `AGENTS.md` and ADR-0011 wording genericized |
| F4 | Okta/Slack/Jira framed as defaults | Reframed across `prd.md`, `hld.md`, `mvp.md`, `product_vision.md`, `pre_development_checklist.md` as generic capabilities with tools as examples; explicit provider-example lists kept |
| F5 | README stale status | Rewritten: accurate status, explicit not-production-ready language, roadmap, versioning policy, trademark attribution |
| F6 | Missing public files | Added `.gitignore`, `CODE_OF_CONDUCT.md` (Contributor Covenant 2.1), `SECURITY.md`, `.github/ISSUE_TEMPLATE/` (bug report, design discussion, config) |
| F7 | Build artifacts in tree | Covered by `.gitignore` (`node_modules`, `dist`, `tsconfig.tsbuildinfo`, `.cache`) |
| F8 | Logo images | Removed by maintainer before publication |
| F9 | Audit document itself contained the scrubbed internal names | This checklist sanitized to record categories, not literal internal identifiers |

## Verification

- `make test`, `make lint`, `make web-test`, and `make fixtures-check` pass
  on the scrubbed tree (run 2026-08-14 after the module path change).
- Final full-tree sweep for internal identifiers (former module path,
  organization and brand names, internal hostnames, real paths) returns
  clean, including this document.

---

# 2. Pre-Publish Checklist (all complete)

## IP and naming

- [x] Project name: **Atlas**, repository **ceph-atlas**, module path
  `github.com/tonymontoya/ceph-atlas`.
- [x] Name-collision search passed: the only `ceph-atlas` repository on
  GitHub is this project's; the web UI package is `private` and never
  published to npm; the Go module path is under the maintainer's account.
- [x] Ceph trademark review completed against Red Hat's published Ceph
  Trademark Use Policy (https://ceph.io/en/trademarks/): the project is
  named Atlas (mark not incorporated into the product name); "Ceph" is used
  nominatively to describe compatibility; the required standard trademark
  attribution statement appears in the README.

## Scrub internal references

- [x] Module path rewritten; test and lint suites re-run.
- [x] Internal organization name removed.
- [x] Environment context genericized and shipped public.
- [x] AGENTS.md wording genericized.
- [x] Okta/Slack/Jira reframed as optional integration examples.
- [x] Logo images removed.
- [x] This checklist sanitized (F9).

## Make the project generic

- [x] Final tree grep for internal identifiers returned clean.
- [x] Fixtures and examples use only `.invalid` / `example.com` domains and
  synthetic identifiers.

## Repository mechanics

- [x] `.gitignore` added, including local LLM/agent tooling state
  directories (`.opencode/`, `.claude/`, `.cursor/`, `.codex/`, `.aider*`).
- [x] Decision recorded: `AGENTS.md` ships public (scrubbed, useful to
  contributors, an emerging open-source convention); only agent tooling
  state is ignored.
- [x] `CODE_OF_CONDUCT.md` added.
- [x] `SECURITY.md` added.
- [x] `.github/ISSUE_TEMPLATE/` added.
- [x] README rewritten with status, roadmap, versioning policy, and
  trademark attribution.
- [x] `AGENTS.md`, `CONTEXT.md`, `docs/adr/`, and `dev-plans/` kept public as
  the project's main value.

## Versioning

- [x] Strategy decided: Semantic Versioning; `0.x` pre-stability; initial
  public tag `v0.1.0`; stability commitments begin at `1.0.0`. Documented in
  README. `v0.1.0` tagged on the initial public commit.

## History safety

- [x] Tree had no git history; public repo created fresh from this tree.
- [x] The internal development repository was never pushed or forked; the
  public repository began as a single fresh initial commit from the
  sanitized tree.
- [x] History export deliberately rejected; revisit only with history
  rewriting tooling if ever needed.

## Publish

- [x] Ownership decided: personal GitHub account (`tonymontoya`) for early
  exploration; move to a dedicated org if traction develops.
- [x] Clean repository created and populated from the sanitized tree
  (2026-08-14), initial commit tagged `v0.1.0`.
- [x] Communications positioned as "early design/prototype work," not
  "ready for users."

---

# 3. Decisions

All decisions from the initial audit are resolved:

1. Project name Atlas, repository `ceph-atlas`, module
   `github.com/tonymontoya/ceph-atlas`.
2. `dev-plans/environment_context.md` ships public, genericized as an
   example environment.
3. Ownership: personal GitHub account initially.
4. `AGENTS.md` ships public; LLM tooling state directories are gitignored.
5. Versioning: SemVer, `v0.1.0` initial tag, stability commitments at
   `1.0.0`.
