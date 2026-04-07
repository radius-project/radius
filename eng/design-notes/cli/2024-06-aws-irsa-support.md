# Radius AWS Workload Identity Support

* **Status**: In Review
* **Author**: Nithya Subramanian (@nithyatsu)

## Overview

A software workload such as a container-based application, service or script needs an identity to authenticate, access, and communicate with services that are distributed across different platforms and/or cloud providers. Radius uses static credentials (Access Key) to interact with AWS IAM (Identity and Access Management) so that we can deploy and access AWS resources today:
 
```
rad credential register aws --access-key-id <access-key-id> --secret-access-key <secret-access-key>
```
 
These IAM credentials, including the access key should be rotated regularly to reduce the chance of unauthorized access. A more secure option than using these credentials is to use IRSA.
 
IRSA (IAM Roles for Service Accounts) is used for authenticating applications running within Kubernetes pods. When we use IRSA, we associate an IAM role directly with a Kubernetes service-account, allowing the pods associated with the service-account to assume that role. IRSA does not require us to configure access key and secret. It relies upon identity tokens (OIDC tokens) to authenticate with AWS services. These tokens have a default expiration (configurable). However, they automatically refresh when needed and are transparent to the user.

The goal of the scenario is to enable infrastructure operators to configure IRSA support for the AWS provider in Radius to deploy and manage AWS resources.

## Terms and definitions
| Term | Definition |
|---|---|
| OIDC | OIDC stands for OpenID Connect. It allows authentication of end-user(or workload). OIDC is used by IRSA to exchange a Kubernetes token for an AWS IAM token. |
|Role ARN| The Role ARN uniquely identifies an AWS IAM Role and can be mapped to a set of permissions. The Role ARN is used to identify the "user" like a ClientID in Azure/Entra or a service account name in Kubernetes.|


## Objectives

> Issue Reference: https://github.com/radius-project/radius/issues/7618 

### Goals

* Radius users can configure AWS provider and enable Radius to use IRSA for authentication and deployment of application.
* IRSA can be configured via interactive experience.
* IRSA can be configured with non-interactive commands.
* Radius users can deploy and manage AWS resources without having to manage AWS access key id and secret

### Non-goals

* Azure Managed Identity support
* Azure Workload Identity support

The above two are addressed in [Azure Workload Identity Support](https://github.com/radius-project/design-notes/blob/main/cli/2024-04-azure-workload-identity.md)

* Ability of radified applications to be able to use IRSA to connect to AWS resources.

This facilitates a user application to be able to configure its container to use IRSA to authenticate itself for communication with AWS. While the technology is IRSA, this feature is very different from enabling Radius to use IRSA to deploy resources. We plan to enable this scenario in the future, but it is not part of this proposal. 

### User scenarios

#### User Story 1

As a user I should be able to configure Radius AWS provider with IRSA through interactive experience.

```
% rad init --full  
Enter an environment name 
>default 

% rad init --full  
Select your cloud provider                     
1. Azure                                  
> 2. AWS                                     
3.[back] 

% rad init --full  

Select the identity for the AWS cloud provider 
1. IAM access key                              
> 2. IAM Role for Service Account (IRSA)

% rad init --full  

Enter the IAM RoleARN for configuring IRSA: <role-arn-radius>
```

#### User story 2

As a user I should be able to configure Radius AWS provider with IRSA using non interactive commands.

1. Install the `rad` CLI.

2. Install Radius using `rad install kubernetes`
    ```
    rad install kubernetes --set global.aws.irsa.enabled=true #default false
    ```
    If enabled, it should print a success/ failure message after updating the pod spec to mount the token. It should also print a information message to the complete of configuration by using rad credential register and rad env update.

3. Create a Radius environment and add an AWS cloud provider.
    ```
    rad group create default

    rad env create default
    
    rad env update default --aws-account-id <aws-account-id> --aws-region <aws-region> 
    ```

4. Register the role ARN using `rad credential register aws irsa`.
    ```
   rad credential register aws irsa --iam-role <roleARN>
    ```

Radius is now configured to use AWS IRSA for authentication.

## Design

### High Level Design for IRSA

Users must choose whether or not to enable IRSA during Radius installation. Then the user should be able to enter the roleARN associated with desired policies which will be stored as the AWS credential.

At a high level, the full flow to setup IRSA looks as below:

1. configure the cluster with oidc provider. 
2. have the role-arn configured with desired policies ready.
3. take steps to allow  `radius-role-arn` and `eks cluster : namespace: service` to trust each other using aws portal.
4. use [interactive](#user-story-1) approach or [non-interactive](#user-story-2) to configure Radius to use IRSA.
5. during rad deploy, radius assumes the role that is configured in the credential. 

OIDC is configured for a cluster and not specific to a role-arn. Therefore, once we have support for multiple credentials in Radius, the same solution would work for supporting multitenancy.

More details about each step is covered in [Detailed Design](#detailed-design)

IRSA works the same way on EKS as well as non-EKS clusters.  

### Architecture Diagram

## User Experience/ CLI Design

Following commands should be updated for AWS IRSA support

`rad init` should be updated to allow configuring a AWS IRSA provider.

`rad credential` command should be updated to register IAM RoleARN.

`rad install kubernetes --set` should be able to enable IRSA. 


**Sample Output:**

n/a

**Sample Recipe Contract:**

n/a

### Detailed Design

#### Proposed Option

Radius should support AWS IRSA.

#### Prerequisite

1. [Setup OIDC provider] (https://docs.aws.amazon.com/eks/latest/userguide/enable-iam-roles-for-service-accounts.html) should be complete for the cluster. 
   
2. `radius-role` with desired policies should be created. Its trust relation should look similar to below. This establishes a trust relation between the service-accounts (ucp and applications-rp) in the specified namespace (radius-system by default) in specified cluster with the specified OIDC provider to be established. 


  ```
  {
    "Version": "2012-10-17",
    "Statement": [
      {
        "Effect": "Allow",
        "Principal": {
          "Federated": "arn:aws:iam::${ACCOUNT_ID}:oidc-provider/${OIDC_PROVIDER}"
        },
        "Action": "sts:AssumeRoleWithWebIdentity",
        "Condition": {
          "StringEquals": {
            "${OIDC_PROVIDER}:aud": "sts.amazonaws.com",
            "${OIDC_PROVIDER}:sub": "system:serviceaccount:<NAMESPACE>:<SERVICE-ACCOUNT>"
          }
        }
      }
    ]
  }
  ```
Example:
  ```
  {
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Principal": {
                "Federated": "arn:aws:iam::817312594854:oidc-provider/oidc.eks.us-west-2.amazonaws.com/id/67DDAC18D8C44CEDCF1C9719A8E9B866"
            },
            "Action": "sts:AssumeRoleWithWebIdentity",
            "Condition": {
                "StringEquals": {
                    "oidc.eks.us-west-2.amazonaws.com/id/67DDAC18D8C44CEDCF1C9719A8E9B866:sub": "system:serviceaccount:radius-system:ucp",
                    "oidc.eks.us-west-2.amazonaws.com/id/67DDAC18D8C44CEDCF1C9719A8E9B866:aud": "sts.amazonaws.com"
                }
            }
        },
    ]
}
  ```

  ```
  {
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Principal": {
                "Federated": "arn:aws:iam::817312594854:oidc-provider/oidc.eks.us-west-2.amazonaws.com/id/67DDAC18D8C44CEDCF1C9719A8E9B866"
            },
            "Action": "sts:AssumeRoleWithWebIdentity",
            "Condition": {
                "StringEquals": {
                    "oidc.eks.us-west-2.amazonaws.com/id/67DDAC18D8C44CEDCF1C9719A8E9B866:sub": "system:serviceaccount:radius-system:applications-rp",
                    "oidc.eks.us-west-2.amazonaws.com/id/67DDAC18D8C44CEDCF1C9719A8E9B866:aud": "sts.amazonaws.com"
                }
            }
        },
    ]
}
  ```



the user-specific details would be {ACCOUNT_ID} and {OIDC_PROVIDER}.


#### Radius support

**installation and setup**

* use `rad install kubernetes --set global.aws.irsa.enabled=true` to add the necessary pod spec to mount the service-account token.

```
    Containers:
      ucp:
        Container ID:   docker://faa5af929f91e053b04b023e05c4fa6363e87688df265443d9f377258fc3fd04
        Image:          ghcr.io/radius-project/ucpd:latest
        Image ID:       docker-pullable://ghcr.io/radius-project/ucpd@sha256:991ed703a3260a19535690242cef4d7e4d076038b016443d9106f2d76d57b0da
        Ports:          9443/TCP, 9090/TCP
        Host Ports:     0/TCP, 0/TCP
        :
        Environment:
          UCP_CONFIG:                   /etc/config/ucp-config.yaml
          BASE_PATH:                    /apis/api.ucp.dev/v1alpha3
          TLS_CERT_DIR:                 /var/tls/cert
          PORT:                         9443
        Mounts:
          /etc/config from config-volume (rw)
          /var/run/secrets/eks.amazonaws.com/serviceaccount from aws-iam-token (ro)
          /var/run/secrets/kubernetes.io/serviceaccount from kube-api-access-d22ld (ro)
          /var/tls/cert from cert (ro)
```

* use `rad credential register aws irsa --iam-role <roleARN>` to register the roleARN that radius would assume for deploying and managing AWS resources.
This command "PUT"s a Radius Credential Resource. As part of this, the credential gets stored as a kubernetes secret (similar to how Radius manages all credentials today).

* use rad env update to store relevant information for deploying resources to accounts and region associated with specific radius environment. (This step is not newly introduced)

```
rad env update qa --aws-account-id <aws-account-id> --aws-region <aws-region> 
```

**post installation**

Once the steps in installation and setup is completed, UCP should be able to "fetch" the configured credentials and then use AssumeRole to manage AWS resources. AWSCredentialProvider should be updated to support the new credential. This can then be used as part of initializing the UCP AWS Module with IRSA credentials. RP should fetch the credential through UCP and utilize it with Terraform provider.  

```
  client := sts.NewFromConfig(cfg)
  assumeRoleArn := retrieveRoleARNCredential() //has logic to get the roleARN stored as credential
  // Assume the new role
  assumeRoleProvider := stscreds.NewAssumeRoleProvider(client, assumeRoleArn)
  // Create a new config with the new credentials
  cfg.Credentials = assumeRoleProvider
  cfg.Region = region
  // Create a new S3 service client
  s3client := s3.NewFromConfig(cfg)
  // Call the ListBuckets function
  output, err := s3client.ListBuckets(context.TODO(), &s3.ListBucketsInput{})
  :

```

Below are the Terraform configurations to be added for IRSA:
Ref: https://registry.terraform.io/providers/hashicorp/aws/latest/docs#assuming-an-iam-role-using-a-web-identity

```
	awsIRSAProvider = "assume_role_with_web_identity"
	awsRoleARN      = "role_arn"
	sessionName     = "session_name"
	tokenFile       = "web_identity_token_file"
	tokenFilePath = "/var/run/secrets/eks.amazonaws.com/serviceaccount/token"
	sessionPrefix = "radius-terraform-"
```

**summary**
1. UCP (or RP) fetches the registered roleARN as AWS Credential.
2. For deploying an AWS resource, it uses this roleARN and [stscreds](https://docs.aws.amazon.com/sdk-for-go/api/aws/credentials/stscreds/) library and constructs an AssumeRoleProvider.
3. assumeRoleProvider communicates with AWS Security Token Service (STS) to assume the specified IAM role.
4. Behind the scenes, 
   1. the service-account token projected as a mounted volume contains the claims for the cluster and service account. AWS STS receives and validates this token. 
   2. If the validation (authentication) succeeds, STS exchanges the service-account token for AWS credentials that can make the AWS API calls.
   3. The IAM role is configured with a trust policy that permits the role to be assumed by the ucp and rp service accounts within the radius-system namespace of the cluster. This configuration enables radius services to provision AWS resources in accordance with the permissions defined by the role.

### API design

we have to add/modify aws-credentials.tsp as below and generate new model that supports AWS IRSA. We should add cli commands and options that utilize the new model.

```
@doc("AWS credential kind")
enum AWSCredentialKind {
  @doc("The AWS Access Key credential")
  AccessKey,

  @doc("The AWS IRSA credential")
  IRSA,
}
```

```
model AwsIRSACredentialProperties extends AwsCredentialProperties {
  @doc("Access Key kind")
  kind: AWSCredentialKind.IRSA;

  @doc("RoleARN for AWS IRSA identity")
  @secret
  roleARN: string;

  @doc("The storage properties")
  storage: CredentialStorageProperties;
}
```
### CLI Design

* `rad init --full` should be updated to ask the user to choose between AWS access keys and IRSA. If IRSA is chosen, the user should be able to input a AWS IAM Role ARN. As a result of rad init, 
   1. the helm charts should be updated with service-account token mounted as secret.
   2. roleARN should be stored as a AWS credential.

* `rad install kubernetes --set global.aws.irsa.enabled=true` should add the necessary pod spec to mount the service-account token.

* `rad credential register aws --access-key-id <access-key-id> --secret-access-key <secret-access-key>` should be replaced with 
`rad credential register aws access-key --access-key-id <access-key-id> --secret-access-key <secret-access-key>`

* `rad credential register aws irsa --iam-role <roleARN>` should be supported.

### Multi-tenancy

Currently, we support only one credential. However, for future, we might want to support multiple credentials potentially across AWS accounts ( say, one for QA team and other for Dev). Once we support multiple credentials, we should be able to assume different roleARN based on which environment we are deploying from, with no changes to AWS IRSA design outlined in this document. We just have to establish right trust entities in all roles.  

### Implementation Details

#### UCP

UCP is responsible for communicating with AWS to deploy AWS resources. 

We should add support for the new Credential type in 
/radius/pkg/ucp/frontend/aws/routes.go - func (m *Module) newAWSConfig(ctx context.Context) (aws.Config, error). 
/radius/pkg/ucp/credentials/aws.go - func (p *AWSCredentialProvider) Fetch(ctx context.Context, planeName, name string) (*AWSCredential, error)

Then UCP should be able to use these to retrieve the stored roleARN and assume it for managing AWS resources.


#### Bicep

NA

#### Deployment Engine

NA

#### Core RP  / Recipes RP

Terraform provider requires AWS credentials in order to deploy recipes.

RP should get Fetch the configured credential through UCP. Then it should use this in the Terraform provider to deploy AWS resources.

### Error Handling

Below errors have to be handled during implementation.
1. handle mismatch of credential type and arguments supplied.
2. what if the user has aws credentials (secrets) configured and wants to switch to IRSA? 
   1. the user has to re-install radius to enable IRSA. This should print informational messages to unregister IRSA credential and re-register the access key credentials.
   2. If there is a mismatch (pod spec has IRSA enabled, but credential type is not role-arn or vice-versa, deployment will fail. Enhance error message to suggest checking the right kind of credential is configured)

## Test plan

We should add a E2E to test the deployment of AWS resources using IRSA. 

## Security

AWS IRSA does not rely of user to rotate credentials for increased security. 
Thus, it should be preferred credential type. User should configure the role associated with Radius setup following principle of least privilege, so that Radius has just the sufficient policies for managing the supported AWS resources.

## Compatibility

This introduces a breaking change to rad credential register command.
We must update samples to use the new rad credential register aws commands.

## Monitoring and Logging

We will have the same monitoring and logging as today. We will not be adding any new instrumentation.

## Development plan

* Create POC for Radius + AWS (1 engineer,1 sprint)  [complete] 
* Create and review technical design (1 engineer, 0.5 sprint) [in progress]
* Implement model changes (1 engineer, 0.5 sprint) 



* Implement changes in UCP, and Recipes RP (1 engineer, 1 sprint)
* Implement CLI and Helm chart changes (1 engineer, 1 sprint) 
* End-to-end testing and documentation (1 engineer, 1 sprint) 



## Open Questions

## Alternatives considered 

### AWS IRSA using EKS Pod Identity Webhook

The [Amazon EKS Pod Identity Webhook](https://github.com/aws/amazon-eks-pod-identity-webhook#amazon-eks-pod-identity-webhook), when installed on the cluster, monitors pods that are associated with a service account which is annotated as follows:

 `eks.amazonaws.com/role-arn: arn:aws:iam::<account-number>:role/<role-name>`

The webhook adds two additional configurations to the relevant pods that get created:
1. Environment variables which the supporting AWS SDK read from automatically to detect IRSA role:
Environment:
        :
        AWS_ROLE_ARN:                 arn:aws:iam::817312594854:role/my-role
        AWS_WEB_IDENTITY_TOKEN_FILE:  /var/run/secrets/eks.amazonaws.com/serviceaccount/token
        :
2. Mount the service-account token as projected volume:
  Volumes:
    aws-iam-token:
      Type:                    Projected (a volume that contains injected data from multiple sources)
    TokenExpirationSeconds:  86400

If we chose to use the web-hook , we could have annotated the ucp and rp service-accounts with roleARN and installed web-hook. However, we did not choose this approach because -
* it requires roleARN to be injected at install time. Dealing with credentials should not require a service restart. 
* when Radius starts supporting multi-tenancy, there is no support in the webhook to handle annotation of service-accounts with multiple role-arns.  

### EKS Pod Identity 

EKS Pod Identity was introduced in 2022 as a simplified approach for applications running on EKS to retrieve credentials. It uses the new EKS pod identity instead of OIDC provider for identity. Because of this, users have convenient APIs that allow managing pod identity. 
Ref. [AWS EKS Pod Identity](https://aws.amazon.com/blogs/containers/amazon-eks-pod-identity-a-new-way-for-applications-on-eks-to-obtain-iam-credentials/). 

Its good to note that since this approach uses its own service principal, the setup involves different identity providers and configuration on aws. Radius should be however able to support this feature with minimal code changes, if any.  


## Design Review Notes

<!--
Update this section with the decisions made during the design review meeting. This should be updated before the design is merged.
-->



