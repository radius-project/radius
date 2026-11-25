#!/bin/bash

# ============================================================================
# Prepare an external PostgreSQL server for Radius (database.external.host).
#
# Creates the ucp, applications_rp, and dynamic_rp users and databases and the
# resources table in each. Safe to re-run; re-running resets the passwords.
#
# The admin connection uses the standard libpq variables (PGHOST, PGPORT,
# PGUSER, PGPASSWORD, PGSSLMODE). RADIUS_DB_PASSWORD is the password for the
# resource provider users; store the same value in the Secret referenced by
# database.external.existingSecret. Requires psql 15 or later.
# ============================================================================

set -euo pipefail

readonly RESOURCE_PROVIDERS=(ucp applications_rp dynamic_rp)
readonly ADMIN_DB="${PGDATABASE:-postgres}"

if [[ -z "${RADIUS_DB_PASSWORD:-}" ]]; then
    echo "Error: RADIUS_DB_PASSWORD is required" >&2
    exit 1
fi

for rp in "${RESOURCE_PROVIDERS[@]}"; do
    echo "Creating user and database ${rp}"
    psql -v ON_ERROR_STOP=1 -v rp="${rp}" --dbname "${ADMIN_DB}" <<'EOSQL'
\getenv pw RADIUS_DB_PASSWORD
SELECT format('CREATE ROLE %I LOGIN', :'rp')
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = :'rp')\gexec
SELECT format('ALTER ROLE %I LOGIN PASSWORD %L', :'rp', :'pw')\gexec
-- Managed servers (for example Azure Database for PostgreSQL) do not grant a
-- superuser, so the admin needs membership to assign ownership and SET ROLE.
GRANT :"rp" TO CURRENT_USER;
SELECT format('CREATE DATABASE %I OWNER %I', :'rp', :'rp')
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = :'rp')\gexec
EOSQL

    echo "Creating resources table in ${rp}"
    psql -v ON_ERROR_STOP=1 -v rp="${rp}" --dbname "${rp}" <<'EOSQL'
-- Azure Database for PostgreSQL makes azure_pg_admin the owner of the public
-- schema, so owning the database does not allow creating tables in it.
GRANT USAGE, CREATE ON SCHEMA public TO :"rp";
SET ROLE :"rp";
CREATE TABLE IF NOT EXISTS resources (
    id TEXT PRIMARY KEY NOT NULL,
    original_id TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    root_scope TEXT NOT NULL,
    routing_scope TEXT NOT NULL,
    etag TEXT NOT NULL,
    created_at TIMESTAMP(6) WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    resource_data JSONB NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_resource_query
    ON resources (resource_type, root_scope);
EOSQL
done

echo "External PostgreSQL is ready for Radius"
