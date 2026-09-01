# The Web UI Adopts IBM Carbon

The Atlas web UI aligns its visual design with the Ceph Dashboard — the interface Ceph operators already live in — rather than inventing its own look. The Ceph Dashboard's design system is IBM Carbon (components, charts, typography) layered on a Bootstrap grid, and Carbon's reference implementation is React, so Atlas adopts `@carbon/react` with IBM Plex fonts natively instead of hand-mimicking the style. Familiarity is the product goal: an operator moving between the Ceph Dashboard and Atlas should experience one visual language for clusters, health, and inventory.

**Considered options**: hand-rolled CSS approximating Carbon's tokens and palette (rejected — re-implementing a maintained design system by hand guarantees drift); Bootstrap plus a custom skin (rejected — matches the Ceph Dashboard only superficially; its tables, notifications, and typography are Carbon's).

**Consequences**

- Carbon's component set (DataTable, notifications, modals, forms, tags) becomes the default vocabulary; deviations need a reason.
- The UI gains a real dependency tree (`@carbon/react`, IBM Plex assets) with its version churn — accepted for a daily-use operator console.
- The retrofit lands with v0.0.8's cluster-index work, when the UI grows its registration and per-cluster surfaces; existing sections are restyled in the same pass.
