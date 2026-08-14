# Scripts

Developer scripts will live here.

Scripts should be thin wrappers around documented commands and must not embed secrets or production endpoints.

- `dev_stack_check.sh` starts the local Docker Compose stack, probes the API and
  web UI, prints compose diagnostics on failure, and stops the stack before
  exiting.
