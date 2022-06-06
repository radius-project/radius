# 003 ADR: Shared Data Store for Radius RP and UCP

## Stakeholders

* Radius Core Team

## Status

accepted

## Context

This ADR documents the need for a shared data store between the Radius RP and UCP.

UCP owns the ResourceGroup resource and implements the CRUD operations on the ResourceGroup. All the Applications.Core/Applications.Connector Resource Provider resources are contained within the resource group but are owned by the RP.

We need to enable querying of all resources within a resource group for supporting deletion of a resourceGroup. However, since the individual resources within the resource group are owned by the RP, the UCP has no knowledge of those unless we have a shared data store.

With a shared data store, UCP can make a query to the data store to return all resources starting with the prefix "/resourceGroups/<rg-name>" and retrieve a list of all resources under the resource group. UCP can then make individual delete requests on each resource which will be processed by the RP.