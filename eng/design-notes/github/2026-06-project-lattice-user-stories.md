# Project Lattice User Journeys

## Priority Definitions

- **p0** -- Critical functionality, MVP feature (blocks launch)
- **p1** -- Core functionality, MVP feature (required for launch but not on the critical path)
- **p2** -- Important functionality, post-MVP (fast-follow features like drift detection, export to Bicep, Google Cloud)
- **p3** -- Nice to have functionality (could ship later or not at all)

## Journey 1: First Deployment

**Goal:** Deploy an application to a cloud environment for the first time.

**Starting State:** A GitHub repository with application source code, Dockerfiles, package manifests (e.g., package.json, go.mod, requirements.txt), and configuration files. No infrastructure-as-code, no Lattice configuration, no environments.

**Ending State:** The application is running in the user's AWS or Azure environment. The Deployment panel shows application resources and their relationships, with links to the source code. It also shows deployed cloud resources with deep links to the cloud provider's console.

**Narrative:** A developer enables Lattice for their repository through the Copilot app. When they ask Copilot to deploy, Copilot prompts them to create a named environment first, walking them through cloud provider details, authorization, and selecting a container platform and registry. The developer provides any secrets or environment-specific configuration values needed at deployment time. Copilot then analyzes the repository and produces an application graph. The developer reviews the graph on the Deployment panel, clicking through resources to see links back to source code. Before deploying, the developer previews what cloud resources will be provisioned, then confirms the deployment. They can track progress on the Deployment panel and are notified when the deployment completes or fails. After deployment, the developer can inspect cloud resource properties, navigate to cloud console pages, and view deployment history. In the future, the developer will also be able to access the app via a public URL or custom domain, view application logs from deployed cloud resources, and show or hide the Deployment panel via a Copilot menu item.

**Key Success Criteria:**

- Lattice is enabled and the repository is connected to the Copilot app
- An environment is created with cloud provider credentials and authorization
- The application graph accurately reflects the application's resources based on the repository's source code
- The developer can preview cloud resources before deploying
- The application deploys successfully and the developer can verify it is running
- Cloud resources are inspectable from the Deployment panel

### User Stories

| # | Pri | User Story | PRFAQ Ref | Owner |
| - | --- | ---------- | --------- | ----- |
| 1 | p0 | As a developer, I can enable Lattice for my repository through the Copilot app so that deployment capabilities are available for my project. | Q10 | TBD |
| 2 | p0 | As a developer, the first time I ask Copilot to deploy an application, I am prompted to create a named environment first. | Q6, Q12 | TBD |
| 3 | p0 | As a developer, when Copilot is creating an environment, Copilot asks me details about my cloud environment such as the AWS account ID and region, or Azure subscription ID and resource group. | Q12 | TBD |
| 4 | p0 | As a developer, when Copilot is creating an environment, Copilot guides me through authorizing GitHub to manage resources within my cloud provider. | Q13, Q23 | TBD |
| 5 | p0 | As a developer, when Copilot is creating an environment, Copilot helps me select a container platform, an OCI registry, and other infrastructure dependencies required by my application. | Q6, Q12 | TBD |
| 8 | p1 | As a developer, I can provide secrets and environment-specific configuration values (e.g., database passwords, API keys) that are stored securely and made available to my application at deployment time. | -- | TBD |
| 9 | p0 | As a developer, I can ask Copilot to analyze my repository and it produces an application graph from my source code, Dockerfiles, and dependencies. | Q6, Q23 | TBD |
| 10 | p0 | As a developer, I can view the application graph on the Deployment panel showing my application's resources, properties of each application resource, and their relationships. | Q6, Q7 | TBD |
| 11 | p0 | As a developer, I can click on any of my application's resources to get a link to the source code that requires the resource (e.g., a container image has a link to a Dockerfile). | Q7, Q23 | TBD |
| 17 | p3 | As a developer, I can easily show and hide the Deployment panel via a Copilot menu item. | Q7 | TBD |
| 18 | p1 | As a developer, before I deploy my application for the first time, I can ask Copilot to preview what cloud resources will be provisioned before deploying to one of my environments. | Q6, Q23 | TBD |
| 19 | p0 | As a developer, I can ask Copilot to deploy my application to one of my environments. I can see deployment progress and status on the Deployment panel. | Q6, Q14 | TBD |
| 20 | p1 | As a developer, I am notified when a deployment completes or fails, even if I have navigated away from the Deployment panel. | -- | TBD |
| 21 | p1 | As a developer, when a deployment fails, I can see the error details on the Deployment panel and understand what went wrong so I can fix and retry. | -- | TBD |
| 22 | p0 | As a developer, I can click a cloud resource and see its properties (e.g., the ARN for an AWS RDS database, a SKU for an Azure database, or a URL for a Kubernetes Service). | Q7, Q23 | TBD |
| 23 | p1 | As a developer, I can click a cloud resource on the application graph to navigate to its cloud console page. | Q7, Q23 | TBD |
| 24 | p1 | As a developer, I can view the history of deployments to an environment, including what was deployed, when, and whether it succeeded or failed. | -- | TBD |
| 25 | p2 | As a developer, after deploying my application, I can access it via a public URL or custom domain. | -- | TBD |
| 26 | p2 | As a developer, I can view application logs from my deployed cloud resources through the Copilot app. | -- | TBD |

## Journey 2: Iterating on an Application

**Goal:** Modify an application and deploy the changes to an existing environment.

**Starting State:** A deployed application with an existing environment and application graph.

**Ending State:** The application graph reflects the changes, the application code and graph are in sync, and the updated application is deployed to the environment.

**Narrative:** A developer needs to evolve their application, for example by adding a database. They ask Copilot to modify the application graph directly (adding, removing, or changing resources and their properties), or they ask Copilot to modify the application code, and Copilot keeps the graph and source code in sync by adding the corresponding resource to the graph and the appropriate client library to the code. When ready, they ask Copilot to deploy the update and are presented with a diff of what will change before approving. In the future, the developer will also be able to edit resource properties directly on the Deployment panel, detect whether the deployed application has drifted from the graph, and either redeploy to match the graph or update the graph to match reality.

**Key Success Criteria:**

- The developer can modify the application graph through Copilot
- Code changes and graph changes stay in sync automatically
- The developer reviews a diff of what will change before approving a deployment

### User Stories

| # | Pri | User Story | PRFAQ Ref | Owner |
| - | --- | ---------- | --------- | ----- |
| 12 | p1 | As a developer, I can ask Copilot to modify my application graph (add, remove, or change resources and their properties) and Copilot updates the graph. | Q11 | TBD |
| 13 | p1 | As a developer, when I ask Copilot to modify my application code (e.g., adding a database), Copilot adds the corresponding resource to the application graph and adds the appropriate database client library in the application source code. Copilot keeps the application code and the application graph in sync. | Q11, Q23 | TBD |
| 14 | p2 | As a developer, I can click on any of my application's resources on the Deployment panel and edit the properties. | Q7 | TBD |
| 27 | p0 | As a developer, after I have made a change to my application graph, I can ask Copilot to update a deployment in one of my environments. I'm presented with a diff of what will change before I approve the deployment. | Q6, Q14 | TBD |
| 33 | p2 | As a developer, I can ask Copilot whether my deployed application has drifted from the application graph. I am shown which resources and which resource properties are different. | Q15, Q24 | TBD |
| 34 | p2 | As a developer, after asking Copilot whether my deployed application has drifted, I can tell Copilot to redeploy to match the application graph. | Q15, Q24 | TBD |
| 35 | p3 | As a developer, after asking Copilot whether my deployed application has drifted, I can tell Copilot to update the graph to match reality. | Q15, Q24 | TBD |

## Journey 3: Promoting Across Environments

**Goal:** Deploy an application to a second environment and manage it across both.

**Starting State:** An application deployed to one environment (e.g., "staging").

**Ending State:** The application is running in a second environment (e.g., "production"). The developer can view both environments, promote between them, and roll back if needed.

**Narrative:** A developer creates a second named environment, going through the same environment creation flow as before, and can view both environments with their cloud provider details, deployed applications, and status. When they are ready, they ask Copilot to promote the application from the first environment to the second, review a diff of what will change, and approve the deployment. If the new deployment breaks the application, they can roll back to a previous version.

**Key Success Criteria:**

- A second environment can be created and configured independently
- The developer can view both environments
- Promotion between environments shows a diff before approval
- Rollback to a previous version is available if a deployment fails

### User Stories

| # | Pri | User Story | PRFAQ Ref | Owner |
| - | --- | ---------- | --------- | ----- |
| 6 | p0 | As a developer, I can ask Copilot to create additional named environments. I am taken through the same environment creation flow. | Q12 | TBD |
| 7 | p1 | As a developer, I can view my existing environments and their cloud provider details, including which applications are deployed, when they were last updated, and whether they have deployment errors. | -- | TBD |
| 28 | p1 | As a developer, I can ask Copilot to promote my deployed application from one environment to another, review the diff of what will change, and approve the deployment. | Q16 | TBD |
| 29 | p1 | As a developer, I can roll back a deployment to a previous version if a new deployment breaks my application. | -- | TBD |

## Journey 4: Collaborating on Changes

**Goal:** Use a pull request workflow to review and approve application changes before deploying.

**Starting State:** A team uses pull requests for code review. A developer has made changes to the application graph on a branch.

**Ending State:** A teammate has reviewed the application changes and approved the PR. The developer can now deploy the approved changes.

**Narrative:** A developer on a team makes a change to the application graph and opens a pull request. Before the change can be deployed, a coworker must approve it. The PR reviewer is presented with a diff of what changed in the application graph. After the reviewer approves the PR, the developer can proceed to deploy. In the future, the reviewer will also be able to ask Copilot to preview how the deployed cloud resources will change in a specific environment before approving.

**Key Success Criteria:**

- Application graph changes require PR approval before deployment
- The PR reviewer sees a clear diff of application graph changes
- Deployment is gated on PR approval

### User Stories

| # | Pri | User Story | PRFAQ Ref | Owner |
| - | --- | ---------- | --------- | ----- |
| 30 | p1 | As a developer on a team that uses pull requests, after I make a change to the application graph, I need my coworker to approve the change before I can deploy to one of my environments. | Q7 | TBD |
| 31 | p1 | As a PR approver, when I'm reviewing a PR, I expect to be presented with a diff of what changed to the application graph. | Q7 | TBD |
| 32 | p2 | As a PR approver, when I'm reviewing a PR, I can ask Copilot to preview how the deployed cloud resources will change in a specific environment once I approve the PR. | Q7, Q23 | TBD |

## Journey 5: Cleaning Up

**Goal:** Remove applications, environments, or Lattice itself when they are no longer needed.

**Starting State:** One or more applications deployed to one or more environments, with Lattice enabled on the repository.

**Ending State:** The targeted applications, environments, or Lattice configuration are fully removed and all associated cloud resources are destroyed.

**Narrative:** A developer decides to clean up. They can ask Copilot to delete an application from a specific environment, reviewing what will be destroyed before confirming. Alternatively, they can delete an application entirely, which removes it from all environments and deletes its definition from the repository. They can also delete an entire environment, which destroys all deployed applications and cloud resources in that environment and removes the environment configuration. In the future, the developer will also be able to cleanly remove Lattice from the repository entirely, including its configuration and workflows.

**Key Success Criteria:**

- The developer reviews what will be destroyed before confirming any deletion
- Deleting an application from one environment does not affect other environments
- Deleting an application entirely removes it from all environments and the repository
- Deleting an environment destroys all of its cloud resources and configuration
- Removing Lattice leaves the repository in a clean state

### User Stories

| # | Pri | User Story | PRFAQ Ref | Owner |
| - | --- | ---------- | --------- | ----- |
| 36 | p1 | As a developer, I can ask Copilot to delete an application from one of my environments, review what will be destroyed, and confirm the deletion. | Q17 | TBD |
| 37 | p1 | As a developer, I can ask Copilot to delete an application entirely, which deletes the application from all of its environments and removes the application graph from my repository. | Q17 | TBD |
| 38 | p1 | As a developer, I can ask Copilot to delete a specific environment, which removes all deployed applications and cloud resources in that environment and deletes the environment configuration. | Q17 | TBD |
| 39 | p2 | As a developer, I can cleanly remove Lattice from my repository, including its configuration and workflows. | -- | TBD |

## Additional Post-MVP Functionality

The following stories are not part of the core user journeys above but represent important post-MVP capabilities.

### User Stories

| # | Pri | User Story | PRFAQ Ref | Owner |
| - | --- | ---------- | --------- | ----- |
| 15 | p2 | As a developer, I can define and deploy multiple independent applications from a single repository (e.g., a monorepo containing several applications that are deployed separately). | -- | TBD |
| 16 | p2 | As a developer, I can deploy applications that span multiple repositories (e.g., a frontend repo and a backend repo) as a single application. | -- | TBD |
| 40 | p2 | As a developer, I can ask Copilot to help me migrate from Lattice to self-hosted Radius. | Q18 | TBD |
| 41 | p2 | As a developer, I can deploy my application to Google Cloud (GKE + managed services). | Q8 | TBD |
| 42 | p2 | As a developer, I can deploy my application's containers to ECS, ACA, or Cloud Run as alternatives to Kubernetes. | Q12 (internal note), Q24 | TBD |
| 43 | p3 | As a developer, I can deploy my application to my local development workstation for testing before deploying to the cloud. | Q9 | TBD |
