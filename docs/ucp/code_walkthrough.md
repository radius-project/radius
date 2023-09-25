## Code Walkthrough

## Entry Point
The entry point for the UCP microservice is cmd/ucpd/main.go. The UCP service is initialized in pkg/ucp/frontend/api/server.go. NewServerOptionsFromEnvironment method in pkg/ucp/server/server.go initializes UCP by looking up the environment variables.

<br/><br/>
## Data Store
The different data stores that UCP supports can be found under pkg/ucp/store. Currently, UCP Data store is shared by the Radius RP. Please refer docs/adr/003-shared-data-store.md for details.

<br/><br/>

## UCP Routes and Controllers
The different routes registered with UCP can be found in pkg/ucp/frontend/api/routes.go. The different controllers that service the routes can be found under pkg/ucp/frontend/controller. The methods that implement CRUDL operations for different UCP resources can be found in the corresponding controller file. 

UCP implements a reverse proxy to forward incoming requests to differents RPs such as Applications.Core under Radius RP. This code can be found in pkg/ucp/frontend/controller/planes/proxyplane.go.

For proxying requests to the AWS plane, UCP needs to perform request translation from ARM-RPC protocol to AWS Cloud control format. This code can be found under pkg/ucp/frontend/controller/awsproxy.

<br/><br/>

## UCP Resource Definitions
The swagger defintions for UCP resources are defined in swagger/specification/ucp/resource-manager/UCP/preview/2023-10-01-preview/ucp.json.

<br/><br/>

## Resource Parsing
pkg/ucp/resources defines generic functions for parsing and constructing resource IDs from URLs.

<br/><br/>

## API Versioning
UCP supports API versioning in a similar way to Radius RP. UCP accepts requests with API versions but converts stores the data in a version agnostic format. A reverse conversion from version agnostic to version aware format is performed for outgoing requests from UCP. This conversion code can be found under pkg/ucp/datamodel and needs to be updated if a new API version is introduced.

