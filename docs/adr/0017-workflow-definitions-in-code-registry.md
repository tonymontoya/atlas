# Workflow Definitions Live in a Code Registry

Workflow definitions — the ordered, typed Job specifications that make up a Workflow such as Replace OSD — are registered in Go code and versioned with the Atlas binary. The database stores only Workflow Instances, which reference a definition id and version. Runtime-authored or admin-editable workflow definitions are deferred until a real consumer exists.

**Consequences**

Job definitions get compile-time type safety and normal code-review governance; adding or changing a Workflow requires an Atlas release. The registry sits behind a small interface so a future DB-backed definition source can be added behind the same seam without reshaping instances. Definition ids and versions referenced by historical instances must remain resolvable as definitions evolve.
