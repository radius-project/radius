#! /usr/bin/env bash
set -e

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

rad resourceprovider create 'Applications.Core' @udt-examples/Applications.Core.json -o json
rad resourceprovider create 'Applications.Dapr' @udt-examples/Applications.Dapr.json -o json
rad resourceprovider create 'Applications.Datastores' @udt-examples/Applications.Datastores.json -o json
rad resourceprovider create 'Applications.Messaging' @udt-examples/Applications.Messaging.json -o json