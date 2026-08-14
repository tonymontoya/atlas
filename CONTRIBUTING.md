# Contributing to Atlas

Atlas is pre-development. Contributions should preserve the product language and architectural decisions before adding implementation.

## Before Opening a Change

1. Read `README.md`.
2. Read `dev-plans/product_vision.md`, `dev-plans/prd.md`, `dev-plans/hld.md`, and `dev-plans/domain_model.md`.
3. Check `docs/adr/` for accepted decisions.
4. Use the terms in `CONTEXT.md` consistently.

## Project Language

Use the canonical Atlas terms:

- Case, not incident, unless referring to an actual operational incident
- Workflow, not playbook
- Job, not command
- Device, not disk, unless referring to physical media
- Timeline Event and Audit Event are different concepts

If a feature requires a new domain concept, update `CONTEXT.md` or `dev-plans/domain_model.md` before implementation.

## Architecture Decisions

Create an ADR in `docs/adr/` when a decision is hard to reverse, surprising without context, and the result of a real tradeoff.

Use short ADRs. Record what was decided and why.

## Development Expectations

When implementation begins, features should include:

- automated tests
- API documentation
- audit coverage for privileged operations
- RBAC checks for protected behavior
- operational logs and metrics
- documentation for users or operators when behavior is user-facing

Do not add generic remote execution, SSH proxying, or arbitrary shell execution to Atlas Agents.
