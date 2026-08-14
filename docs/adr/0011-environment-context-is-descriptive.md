# Environment Context Is Descriptive

Atlas may document an example operating environment to sharpen MVP requirements, but environment-specific facts are descriptive input, not product constraints. Atlas must remain a general Ceph operations platform and must not hard-code one organization's names, storage families, tooling, network topology, identity system, Slack channels, Jira project, or repository layout into its core model.

**Consequences**

Environment context can justify adapters, examples, pilot defaults, and migration notes. Core domain models, APIs, workflow semantics, agent operations, and authorization concepts must stay portable across Ceph operators.
