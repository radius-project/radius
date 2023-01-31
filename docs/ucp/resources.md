## UCP Resources

UCP supports CRUDL operations for the following resources:-
### Plane
UCP uses a Plane resource to support ids that come from different types of systems (Azure vs GCP) or different instances of those systems (Azure Cloud vs Azure Gov Cloud).

### Resource Group
A resource group is used to organize user resources. Note that even though conceptually this is similar to an Azure resource group but it is not the same and is a UCP resource independent of Azure.

### Credentials
A user can configure provider credentials in UCP. Currently Azure and AWS credentials are supported. Please refer to [Credential Design Document](https://microsoft.sharepoint.com/:w:/t/radiuscoreteam/EVAuQrRK6tRIqiOZmjnyxjoBUfaa2jF2uiV-jhibg5qB5A?e=2t2hef) for details.
