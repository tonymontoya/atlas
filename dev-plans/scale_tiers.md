# Atlas Scale Tiers
**Version:** 0.1 (Draft)  
**Status:** Pre-development Planning Document  
**Audience:** Engineering, Architecture, Product, Contributors  
**Project:** Atlas

---

# 1. Purpose

This document converts Atlas scale ambitions into named validation tiers.

The high-end enterprise targets remain long-term architecture goals. MVP development should validate smaller tiers first.

---

# 2. Tier 0: Developer

Used for local development and CI.

| Metric | Target |
|---------|---------|
| Zones | 1 |
| Datacenters | 1 |
| Clusters | 1 |
| Hosts | 3-10 |
| Devices | 10-100 |
| OSDs | 10-100 |
| Concurrent Users | 1-5 |
| Concurrent Workflows | 1-5 |
| Cases | 100-1,000 |

---

# 3. Tier 1: MVP Pilot

Used to validate the single-zone MVP.

| Metric | Target |
|---------|---------|
| Zones | 1 |
| Datacenters | 1-3 |
| Clusters | 1-5 |
| Hosts | 100-1,000 |
| Devices | 1,000-20,000 |
| OSDs | 1,000-20,000 |
| Concurrent Users | 10-100 |
| Concurrent Workflows | 10-100 |
| Cases | 10,000-100,000 |

---

# 4. Tier 2: Single-Zone Production

Used to validate a production regional Atlas deployment before federation.

| Metric | Target |
|---------|---------|
| Zones | 1 |
| Datacenters | 3-25 |
| Clusters | 5-50 |
| Hosts | 1,000-10,000 |
| Devices | 20,000-200,000 |
| OSDs | 20,000-200,000 |
| Concurrent Users | 100-1,000 |
| Concurrent Workflows | 100-5,000 |
| Cases | 100,000-1,000,000 |

---

# 5. Tier 3: Enterprise Federation

Used after global coordination and multi-zone synchronization exist.

| Metric | Target |
|---------|---------|
| Zones | 5-50 |
| Datacenters | 25-500 |
| Clusters | 50-500 |
| Hosts | 10,000-100,000 |
| Devices | 200,000-2,000,000 |
| OSDs | 200,000-1,000,000 |
| Concurrent Users | 1,000-5,000 |
| Concurrent Workflows | 5,000-50,000 |
| Cases | practical unlimited retention |

---

# 6. Validation Rules

Every performance, load, or retention claim should name its scale tier.

MVP acceptance should target Tier 0 and Tier 1.

Single-zone production readiness should target Tier 2.

Federated architecture validation should target Tier 3.
