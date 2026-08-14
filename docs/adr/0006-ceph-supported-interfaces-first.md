# Ceph Supported Interfaces First

Atlas will integrate with Ceph through supported APIs first, supported CLI second, and an optional Atlas ceph-mgr module later. This keeps Atlas compatible with upstream Ceph expectations and avoids coupling the MVP to unsupported internal implementation details.

**Consequences**

Each Ceph operation should document the supported interface it uses and the Ceph versions it targets. CLI-based operations must be wrapped behind typed provider interfaces so they can be replaced by APIs or a future mgr module.
