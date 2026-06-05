# Project Lattice User Story Inventory

June 5, 2026\
zachcasper

## Priority Definitions

- **p0** -- Critical functionality, MVP feature (blocks launch)
- **p1** -- Core functionality, MVP feature (required for launch but not on the critical path)
- **p2** -- Important functionality, post-MVP (fast-follow features like drift detection, export to Bicep, Google Cloud)
- **p3** -- Nice to have functionality (could ship later or not at all)

## Getting started and creating environments

| # | Pri | User Story | PRFAQ Ref | Owner |
| - | --- | ---------- | --------- | ----- |
| 1 | p0 | As a developer, I can enable Lattice for my repository through the Copilot app so that deployment capabilities are available for my project. | Q10 | TBD |
| 2 | p0 | As a developer, the first time I ask Copilot to deploy an application, I am prompted to create a named environment first. | Q6, Q12 | TBD |
| 3 | p0 | As a developer, when Copilot is creating an environment, Copilot asks me details about my cloud environment such as the AWS account ID and region, or Azure subscription ID and resource group. | Q12 | TBD |
| 4 | p0 | As a developer, when Copilot is creating an environment, Copilot guides me through authorizing GitHub to manage resources within my cloud provider. | Q13, Q23 | TBD |
| 5 | p0 | As a developer, when Copilot is creating an environment, Copilot helps me select a container platform, an OCI registry, and other infrastructure dependencies required by my application. | Q6, Q12 | TBD |
| 6 | p0 | As a developer, I can ask Copilot to create additional named environments. I am taken through the same environment creation flow. | Q12 | TBD |
| 7 | p1 | As a developer, I can view my existing environments and their cloud provider details, including which applications are deployed, when they were last updated, and whether they have deployment errors. | -- | TBD |
| 8 | p1 | As a developer, I can provide secrets and environment-specific configuration values (e.g., database passwords, API keys) that are stored securely and made available to my application at deployment time. | -- | TBD |

## Defining and editing an application

| # | Pri | User Story | PRFAQ Ref | Owner |
| - | --- | ---------- | --------- | ----- |
| 9 | p0 | As a developer, I can ask Copilot to analyze my repository and it produces an application graph from my source code, Dockerfiles, and dependencies. | Q6, Q23 | TBD |
| 10 | p0 | As a developer, I can view the application graph on the Deployment panel showing my application's resources, properties of each application resource, and their relationships. | Q6, Q7 | TBD |
| 11 | p0 | As a developer, I can click on any of my application's resources to get a link to the source code that requires the resource (e.g., a container image has a link to a Dockerfile). | Q7, Q23 | TBD |
| 12 | p1 | As a developer, I can ask Copilot to modify my application graph (add, remove, or change resources and their properties) and Copilot updates the graph. | Q11 | TBD |
| 13 | p2 | As a developer, I can click on any of my application's resources on the Deployment panel and edit the properties. | Q7 | TBD |
| 14 | p2 | As a developer, I can deploy applications that span multiple repositories (e.g., a frontend repo and a backend repo) as a single application. | -- | TBD |
| 15 | p3 | As a developer, I can easily show and hide the Deployment panel via a Copilot menu item. | Q7 | TBD |

## Previewing and deploying a new application

| # | Pri | User Story | PRFAQ Ref | Owner |
| - | --- | ---------- | --------- | ----- |
| 16 | p1 | As a developer, before I deploy my application for the first time, I can ask Copilot to preview what cloud resources will be provisioned before deploying to one of my environments. | Q6, Q23 | TBD |
| 17 | p0 | As a developer, I can ask Copilot to deploy my application to one of my environments. I can see deployment progress and status on the Deployment panel. | Q6, Q14 | TBD |
| 18 | p1 | As a developer, I am notified when a deployment completes or fails, even if I have navigated away from the Deployment panel. | -- | TBD |
| 19 | p1 | As a developer, when a deployment fails, I can see the error details on the Deployment panel and understand what went wrong so I can fix and retry. | -- | TBD |
| 20 | p0 | As a developer, I can click a cloud resource and see its properties (e.g., the ARN for an AWS RDS database, a SKU for an Azure database, or a URL for a Kubernetes Service). | Q7, Q23 | TBD |
| 21 | p1 | As a developer, I can click a cloud resource on the application graph to navigate to its cloud console page. | Q7, Q23 | TBD |
| 22 | p1 | As a developer, I can view the history of deployments to an environment, including what was deployed, when, and whether it succeeded or failed. | -- | TBD |
| 23 | p2 | As a developer, after deploying my application, I can access it via a public URL or custom domain. | -- | TBD |
| 24 | p2 | As a developer, I can view application logs from my deployed cloud resources through the Copilot app. | -- | TBD |

## Modifying an application and rolling out changes

| # | Pri | User Story | PRFAQ Ref | Owner |
| - | --- | ---------- | --------- | ----- |
| 25 | p0 | As a developer, after I have made a change to my application definition, I can ask Copilot to update a deployment in one of my environments. I'm presented with a diff of what will change before I approve the deployment. | Q6, Q14 | TBD |
| 26 | p1 | As a developer, I can ask Copilot to promote my deployed application from one environment to another, review the diff of what will change, and approve the deployment. | Q16 | TBD |
| 27 | p1 | As a developer, I can roll back a deployment to a previous version if a new deployment breaks my application. | -- | TBD |
| 28 | p1 | As a developer on a team that uses pull requests, after I make a change to the application definition, I need my coworker to approve the change before I can deploy to one of my environments. | Q7 | TBD |
| 29 | p1 | As a PR approver, when I'm reviewing a PR, I expect to be presented with a diff of what changed to the application definition. | Q7 | TBD |
| 30 | p2 | As a PR approver, when I'm reviewing a PR, I can ask Copilot to preview how the deployed cloud resources will change in a specific environment once I approve the PR. | Q7, Q23 | TBD |

## Detecting drift

| # | Pri | User Story | PRFAQ Ref | Owner |
| - | --- | ---------- | --------- | ----- |
| 31 | p2 | As a developer, I can ask Copilot whether my deployed application has drifted from the application graph. I am shown which resources and which resource properties are different. | Q15, Q24 | TBD |
| 32 | p2 | As a developer, after asking Copilot whether my deployed application has drifted, I can tell Copilot to redeploy to match the application graph. | Q15, Q24 | TBD |
| 33 | p3 | As a developer, after asking Copilot whether my deployed application has drifted, I can tell Copilot to update the graph to match reality. | Q15, Q24 | TBD |

## Deleting and tearing down

| # | Pri | User Story | PRFAQ Ref | Owner |
| - | --- | ---------- | --------- | ----- |
| 34 | p1 | As a developer, I can ask Copilot to delete an application from one of my environments, review what will be destroyed, and confirm the deletion. | Q17 | TBD |
| 35 | p1 | As a developer, I can ask Copilot to delete an application entirely, which deletes the application from all of its environments and removes the application definition from my repository. | Q17 | TBD |
| 36 | p1 | As a developer, I can ask Copilot to delete a specific environment, which removes all deployed applications and cloud resources in that environment and deletes the environment configuration. | Q17 | TBD |
| 37 | p2 | As a developer, I can cleanly remove Lattice from my repository, including its configuration and workflows. | -- | TBD |

## Additional post-MVP functionality

| # | Pri | User Story | PRFAQ Ref | Owner |
| - | --- | ---------- | --------- | ----- |
| 38 | p2 | As a developer, I can ask Copilot to help me migrate from Lattice to self-hosted Radius. | Q18 | TBD |
| 39 | p2 | As a developer, I can deploy my application to Google Cloud (GKE + managed services). | Q8 | TBD |
| 40 | p2 | As a developer, I can deploy my application's containers to ECS, ACA, or Cloud Run as alternatives to Kubernetes. | Q12 (internal note), Q24 | TBD |
| 41 | p3 | As a developer, I can deploy my application to my local development workstation for testing before deploying to the cloud. | Q9 | TBD |
