name: Bug report
description: Report something broken in the Atlas scaffold, API, docs, or local development stack
labels: ["bug"]
body:
  - type: textarea
    id: description
    attributes:
      label: Description
      description: A clear description of the problem.
    validations:
      required: true
  - type: textarea
    id: reproduction
    attributes:
      label: Steps to reproduce
      placeholder: |
        1. make dev-stack-up
        2. curl http://127.0.0.1:8080/api/v1/...
    validations:
      required: true
  - type: textarea
    id: expected
    attributes:
      label: Expected behavior
    validations:
      required: true
  - type: textarea
    id: actual
    attributes:
      label: Actual behavior
      description: Include logs or error output where relevant.
    validations:
      required: true
  - type: textarea
    id: environment
    attributes:
      label: Environment
      placeholder: OS, Go version, Node version, Docker/Podman version, relevant ATLAS_* overrides
