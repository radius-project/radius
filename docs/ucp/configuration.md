## UCP Configuration

UCP is configured to communicate with the different planes that it supports, currently Radius RP, Deployment Engine and AWS. Note: We will eventually add Azure to this list for which the communication currently happens via Deployment engine.

The configuration can be found in: deploy/Chart/charts/ucp/ucp-config.yaml.

Within each plane, the configuration specifies a URL to communicate with every supported resource provider. For example, separate URLs are specified for Applications.Core and portable resource providers within the Radius plane.
