#!/bin/bash
set -e

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

# Array of usernames
RESOURCE_PROVIDERS=("ucp" "applications_rp")

# Create databases and users
for RESOURCE_PROVIDER in "${RESOURCE_PROVIDERS[@]}"; do
    echo "Creating database and user for $RESOURCE_PROVIDER"
    psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
        CREATE USER $RESOURCE_PROVIDER WITH PASSWORD '$POSTGRES_PASSWORD';
        CREATE DATABASE $RESOURCE_PROVIDER;
        GRANT ALL PRIVILEGES ON DATABASE $RESOURCE_PROVIDER TO $RESOURCE_PROVIDER;
EOSQL
done

# Create tables within those databases
for RESOURCE_PROVIDER in "${RESOURCE_PROVIDERS[@]}"; do
    echo "Creating tables in database $RESOURCE_PROVIDER"
    psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$RESOURCE_PROVIDER" < $SCRIPT_DIR/db.sql.txt
done