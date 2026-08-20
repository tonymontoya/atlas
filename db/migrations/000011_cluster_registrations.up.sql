BEGIN;

-- Cluster Registration (ADR-0025/0026): an Operator registers a Cluster
-- before any observation exists. fsid is bound later, at Enrollment, and
-- ceph_version is learned at first sync, so both become nullable. The
-- existing UNIQUE constraint on fsid already rejects duplicate bindings
-- (NULLs never conflict).
ALTER TABLE atlas_clusters
    ALTER COLUMN fsid DROP NOT NULL,
    ALTER COLUMN ceph_version DROP NOT NULL;

-- Registration bookkeeping on the cluster row itself. NULL registered_at
-- marks rows created by inventory sync before registration existed.
ALTER TABLE atlas_clusters
    ADD COLUMN registered_at TIMESTAMPTZ,
    ADD COLUMN registered_by TEXT,
    ADD COLUMN deregistered_at TIMESTAMPTZ;

-- One-time Enrollment Credentials: generated at registration, stored
-- only as a SHA-256 hash, single-use (consumed_at), expiring. Destructive
-- revocation happens by consuming every live credential at deregistration.
CREATE TABLE cluster_enrollment_credentials (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    cluster_id BIGINT NOT NULL REFERENCES atlas_clusters(id) ON DELETE CASCADE,
    credential_hash TEXT NOT NULL UNIQUE CHECK (credential_hash <> ''),
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX cluster_enrollment_credentials_live_idx
    ON cluster_enrollment_credentials (cluster_id)
    WHERE consumed_at IS NULL;

COMMIT;
