ALTER TABLE device_authorizations ADD COLUMN IF NOT EXISTS scope TEXT NOT NULL DEFAULT 'inventory.read';
ALTER TABLE access_grants ADD COLUMN IF NOT EXISTS scope TEXT NOT NULL DEFAULT 'inventory.read';
