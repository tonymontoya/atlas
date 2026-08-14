# Modular Monolith First

Atlas will start as a modular monolith with strict internal module boundaries rather than a set of distributed microservices. The product needs strong consistency across cases, workflows, policy, RBAC, and audit early on, and a modular monolith keeps development and deployment simpler while the domain model is still stabilizing.

**Consequences**

Modules should own their domain models and expose explicit internal interfaces. Splitting a module into a separate service is a later deployment decision, not a reason to introduce network boundaries during the MVP.
