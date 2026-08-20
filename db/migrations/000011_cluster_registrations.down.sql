BEGIN;

DROP TABLE cluster_enrollment_credentials;

ALTER TABLE atlas_clusters
    DROP COLUMN deregistered_at,
    DROP COLUMN registered_by,
    DROP COLUMN registered_at;

-- Registered-but-never-observed clusters have no fsid or version; drop
-- them before restoring the NOT NULL constraints they violated.
DELETE FROM atlas_clusters WHERE fsid IS NULL;

UPDATE atlas_clusters SET ceph_version = '' WHERE ceph_version IS NULL;

ALTER TABLE atlas_clusters
    ALTER COLUMN fsid SET NOT NULL,
    ALTER COLUMN ceph_version SET NOT NULL;

COMMIT;
