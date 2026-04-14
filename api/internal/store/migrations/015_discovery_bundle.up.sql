-- Seed a global "discovery" bundle row so scan_type=discovery scans have
-- a real bundles.id to point at. The agent never fetches it — its
-- bundle_name/version on the directive is nominal (ADR 003 R1a) — but
-- scans.bundle_id is NOT NULL REFERENCES bundles(id), so we need a row.
--
-- Well-known id keeps the UI's "Start discovery scan" launcher simple:
-- it always passes this id and never asks the user to pick a bundle.
INSERT INTO bundles (id, tenant_id, name, version, framework, target_type)
VALUES (
    '11111111-1111-1111-1111-111111111111',
    NULL,
    'discovery',
    '1.0.0',
    'recon',
    'network_range'
)
ON CONFLICT (name, version) DO NOTHING;
