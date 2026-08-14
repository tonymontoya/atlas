# Implementation Stack

Atlas will use Go for the backend and Atlas Agent, React with TypeScript for the Web UI, PostgreSQL for durable persistence, Redis for cache use cases, NATS for messaging, PostgreSQL full-text search initially, OIDC for authentication, REST API v1 for public APIs, Kubernetes or Podman for deployment, and OCI containers for packaging. This stack fits Atlas because Go is a strong fit for operational control-plane software and agents, PostgreSQL provides reliable transactional state for workflows and audit, and React with TypeScript gives the UI a mature typed frontend foundation.

**Consequences**

Initial repository layout, build tooling, CI, local development topology, and deployment manifests should assume this stack. Replacing Go, PostgreSQL, REST API v1, or the container packaging model would be a significant architecture change and requires a new ADR.
