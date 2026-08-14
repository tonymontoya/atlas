# Scale By Deployment Tier

Atlas will use explicit deployment scale tiers instead of treating the largest enterprise targets as MVP acceptance criteria. The architecture should keep the large-fleet goal visible, but development needs smaller validation steps for lab, single-zone production, enterprise zone, and future federation.

**Consequences**

Performance tests, data retention, and operational limits should name the tier they validate. MVP success is measured against the lab and single-zone production tiers, not the long-term global-fleet target.
