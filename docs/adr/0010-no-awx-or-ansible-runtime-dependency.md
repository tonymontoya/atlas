# No AWX Or Ansible Runtime Dependency

Atlas must not depend on AWX or Ansible as its runtime execution layer. Existing AWX jobs, Ansible playbooks, and operational runbooks may be used as source material for workflow discovery, safety checks, and migration planning, but Atlas workflows must execute through Atlas-owned interfaces and Atlas Agents without requiring AWX or Ansible to be present.

**Consequences**

Atlas Agents must implement typed operation providers directly or through replaceable adapters that are not tied to AWX or Ansible. Any compatibility bridge to current AWX/Ansible workflows must be optional, temporary, and clearly marked as migration support rather than core architecture.
