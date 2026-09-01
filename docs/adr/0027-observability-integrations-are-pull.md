# Third-Party Observability Integrations Are Pull

ADR-0025's rule — Atlas stores no credentials for, and initiates no connections into, the Ceph cluster trust domain — is scoped to Ceph clusters and their agents. Observability and logging systems are a different integration category: deployments already run a monitoring environment (Prometheus, later Datadog; OpenSearch/Splunk for logs), those systems cannot be expected to dial into Atlas, and their data is read-only context rather than a privileged path into cluster control. Atlas therefore pulls from them using integration credentials held in Atlas configuration, starting with a real Prometheus `ObservabilityProvider` reading `/api/v1/alerts` to create Cases through the existing detection pipeline.

Alerts do not flow through the Atlas Agent: relaying them would couple alert delivery to cluster enrollment and split one concern across two channels, and Alertmanager-style webhooks into Atlas would create un-enrolled, bearer-secret senders — the standing-access pattern ADR-0026 rejects for agents. The Agent channel stays dedicated to the Ceph cluster trust domain (ADR-0025).

**Considered options**: Agent-relayed alerts (rejected above); Alertmanager webhooks pushing to Atlas (rejected — second un-enrolled trust relationship with a copyable secret); Atlas pulling Prometheus (chosen).

**Consequences**

- Integration credentials (a Prometheus URL plus optional authentication) live in Atlas configuration; their storage and rotation story joins the security checklist alongside the CA key, distinct from the forbidden cluster-credential class.
- Alerts join clusters by their `cluster` label resolved to an FSID (`resolveClusterLabel`), so one environment-level Prometheus can serve multiple registered clusters.
- The `alert_evaluation_runs.provider` check must widen beyond `'fake'`, and alert-source selection gains its own explicit opt-in (mirroring `ATLAS_PROVIDER_MODE`), so local development and CI stay fake-first.
- Future integrations (Datadog, OpenSearch, Splunk) follow this same pull pattern rather than inventing per-vendor push arrangements.
- Alertmanager integration (silence-aware detection, silence context on Cases) is deferred: its value is policy behavior, which joins the v0.0.9 policy-evaluation line rather than the v0.0.8 alert source. It would slot in as another `ATLAS_ALERT_SOURCE` value.
