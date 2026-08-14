# Durable Events Before Automation

Atlas will treat meaningful operational events as durable records before using them to trigger automation. Event delivery can use a message bus, but the authoritative event history must survive broker restarts and support replay or reconciliation.

**Consequences**

The MVP can start with a simple event table and transactional publishing pattern before adopting a larger event architecture. Triggers should be written so missed broker delivery does not permanently lose operational work.
