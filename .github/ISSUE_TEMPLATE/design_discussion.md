name: Design discussion
description: Propose or discuss a design change before implementation
labels: ["design"]
body:
  - type: textarea
    id: problem
    attributes:
      label: Problem
      description: What operational or design problem does this address?
    validations:
      required: true
  - type: textarea
    id: proposal
    attributes:
      label: Proposal
    validations:
      required: true
  - type: textarea
    id: alternatives
    attributes:
      label: Alternatives considered
  - type: dropdown
    id: affects
    attributes:
      label: Area affected
      options:
        - Domain model / language (CONTEXT.md, domain_model.md)
        - Architecture decision (needs an ADR)
        - Provider contracts
        - REST API (api/openapi)
        - Database schema (db/migrations)
        - Web UI
        - Local development workflow
        - Other / not sure
    validations:
      required: true
