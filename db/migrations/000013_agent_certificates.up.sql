BEGIN;

-- Durable record of issued Agent client certificates (ADR-0026).
-- Enrollment exchanges a one-time Enrollment Credential for a
-- certificate chained to the Atlas internal CA; the serial number maps
-- a certificate to exactly one registered Cluster, which is also how
-- v0.0.8 revocation works: Atlas rejects the certificate (revoked_at is
-- set) instead of maintaining a CRL.

CREATE TABLE cluster_agent_certificates (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    cluster_id BIGINT NOT NULL REFERENCES atlas_clusters(id) ON DELETE CASCADE,
    serial_number TEXT NOT NULL UNIQUE CHECK (serial_number <> ''),
    fingerprint TEXT NOT NULL CHECK (fingerprint <> ''),
    common_name TEXT NOT NULL CHECK (common_name <> ''),
    not_before TIMESTAMPTZ NOT NULL,
    not_after TIMESTAMPTZ NOT NULL,
    issued_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ,
    CHECK (not_after > not_before)
);

CREATE INDEX cluster_agent_certificates_cluster_idx
    ON cluster_agent_certificates (cluster_id);

COMMIT;
