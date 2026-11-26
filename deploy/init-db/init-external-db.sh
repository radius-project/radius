#!/bin/bash

# ============================================================================
# Prepare an external PostgreSQL server for Radius (database.external.host).
#
# Creates the ucp, applications_rp, and dynamic_rp databases, the resources
# table in each, and the users that database.external.authMode needs. Safe to
# re-run. Requires psql 15 or later.
#
# The admin connection uses the standard libpq variables (PGHOST, PGPORT,
# PGUSER, PGPASSWORD, PGSSLMODE). RADIUS_DB_AUTH_MODE selects the mode:
#
#   password (default): creates the ucp, applications_rp, and dynamic_rp
#     users. RADIUS_DB_PASSWORD is their password; store the same value in the
#     Secret referenced by database.external.existingSecret. Re-running resets
#     the passwords.
#
#   azure: Azure Database for PostgreSQL with Microsoft Entra authentication.
#     Creates one role, RADIUS_DB_AZURE_USER (database.external.azure.user),
#     mapped to the identity whose object (principal) ID is
#     RADIUS_DB_AZURE_OBJECT_ID, and grants it access to the three databases.
#     Connect as a Microsoft Entra administrator of the server, with an access
#     token as the password:
#       PGUSER=<administrator name>
#       PGPASSWORD="$(az account get-access-token --resource-type oss-rdbms \
#         --query accessToken --output tsv)"
# ============================================================================

set -euo pipefail

readonly RESOURCE_PROVIDERS=(ucp applications_rp dynamic_rp)
readonly ADMIN_DB="${PGDATABASE:-postgres}"
readonly AUTH_MODE="${RADIUS_DB_AUTH_MODE:-password}"

# The resources table, created in each resource provider database. It is
# schema-qualified so the admin's search_path can't place it elsewhere.
readonly CREATE_TABLE_SQL='
CREATE TABLE IF NOT EXISTS public.resources (
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
    ON public.resources (resource_type, root_scope);
'

lowercase() {
    tr '[:upper:]' '[:lower:]' <<<"$1"
}

init_password_mode() {
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
        psql -v ON_ERROR_STOP=1 -v rp="${rp}" --dbname "${rp}" <<EOSQL
-- Azure Database for PostgreSQL makes azure_pg_admin the owner of the public
-- schema, so owning the database does not allow creating tables in it.
GRANT USAGE, CREATE ON SCHEMA public TO :"rp";
SET ROLE :"rp";
${CREATE_TABLE_SQL}
EOSQL
    done
}

init_azure_mode() {
    if [[ -n "${RADIUS_DB_PASSWORD:-}" ]]; then
        echo "Error: RADIUS_DB_PASSWORD is not used with RADIUS_DB_AUTH_MODE=azure; unset it" >&2
        exit 1
    fi
    local user="${RADIUS_DB_AZURE_USER:-}"
    local object_id="${RADIUS_DB_AZURE_OBJECT_ID:-}"
    if [[ -z "${user}" || -z "${object_id}" ]]; then
        echo "Error: RADIUS_DB_AZURE_USER and RADIUS_DB_AZURE_OBJECT_ID are required with RADIUS_DB_AUTH_MODE=azure" >&2
        exit 1
    fi

    # An existing role must already map to the same identity: re-running must
    # not silently keep a role that signs in as someone else.
    local role_exists mapped_id
    role_exists="$(psql -v ON_ERROR_STOP=1 -tA -v user="${user}" --dbname "${ADMIN_DB}" <<'EOSQL'
SELECT EXISTS (SELECT FROM pg_roles WHERE rolname = :'user');
EOSQL
)"
    mapped_id="$(psql -v ON_ERROR_STOP=1 -tA -v user="${user}" --dbname "${ADMIN_DB}" <<'EOSQL'
SELECT objectId FROM pg_catalog.pgaadauth_list_principals(false) WHERE rolename = :'user';
EOSQL
)"
    if [[ "${role_exists}" == "t" ]]; then
        if [[ -z "${mapped_id}" ]]; then
            echo "Error: role ${user} exists but is not a Microsoft Entra principal" >&2
            exit 1
        fi
        if [[ "$(lowercase "${mapped_id}")" != "$(lowercase "${object_id}")" ]]; then
            echo "Error: role ${user} is mapped to object ID ${mapped_id}, not ${object_id}" >&2
            exit 1
        fi
        echo "Role ${user} already maps to ${object_id}"
    else
        echo "Creating role ${user} for object ID ${object_id}"
        psql -v ON_ERROR_STOP=1 -v user="${user}" -v oid="${object_id}" --dbname "${ADMIN_DB}" <<'EOSQL'
SELECT * FROM pg_catalog.pgaadauth_create_principal_with_oid(:'user', :'oid', 'service', false, false);
EOSQL
    fi

    for rp in "${RESOURCE_PROVIDERS[@]}"; do
        echo "Creating database ${rp}"
        psql -v ON_ERROR_STOP=1 -v rp="${rp}" -v user="${user}" --dbname "${ADMIN_DB}" <<'EOSQL'
SELECT format('CREATE DATABASE %I', :'rp')
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = :'rp')\gexec
GRANT CONNECT ON DATABASE :"rp" TO :"user";
EOSQL

        # A table created in password mode is owned by the password user, so
        # the admin can grant on it only if it has that role's privileges.
        local owner
        owner="$(psql -v ON_ERROR_STOP=1 -tA --dbname "${rp}" <<'EOSQL'
SELECT tableowner FROM pg_tables
WHERE schemaname = 'public' AND tablename = 'resources'
    AND NOT pg_has_role(current_user, tableowner, 'USAGE');
EOSQL
)"
        if [[ -n "${owner}" ]]; then
            echo "Error: public.resources in ${rp} is owned by ${owner}, whose privileges the admin doesn't have; grant ${owner} to the admin or use new databases" >&2
            exit 1
        fi

        # The admin owns the table; the role gets only the statements Radius
        # runs and can't create objects.
        echo "Creating resources table in ${rp}"
        psql -v ON_ERROR_STOP=1 -v user="${user}" --dbname "${rp}" <<EOSQL
${CREATE_TABLE_SQL}
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
GRANT USAGE ON SCHEMA public TO :"user";
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.resources TO :"user";
EOSQL
    done
}

case "${AUTH_MODE}" in
    password) init_password_mode ;;
    azure) init_azure_mode ;;
    *)
        echo "Error: RADIUS_DB_AUTH_MODE must be password or azure, got ${AUTH_MODE}" >&2
        exit 1
        ;;
esac

echo "External PostgreSQL is ready for Radius"
