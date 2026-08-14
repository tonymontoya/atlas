# Security Policy

## Supported Versions

Atlas is pre-release, early-stage software. No versions are currently
supported for production use. Security fixes are applied to the default
branch only.

## Reporting a Vulnerability

Report vulnerabilities privately through GitHub's private vulnerability
reporting for this repository (Security tab -> Report a vulnerability), or by
contacting the maintainer directly via GitHub (https://github.com/tonymontoya).

Please do not open public issues for suspected vulnerabilities. Include
reproduction steps and affected components where possible. You will receive an
acknowledgment within a few days.

## Scope Notes

- Atlas currently exposes a read-only REST API scaffold and a fake inventory
  provider only. No Atlas Agent, mutation surface, authentication, or RBAC
  enforcement exists yet.
- Design constraints for future privileged operations are recorded in
  `docs/adr/0003-agent-typed-operations-only.md` and
  `dev-plans/security_review_checklist.md`.
