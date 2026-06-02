# GitHub and Azure Incubations Make Cloud Deployment a Conversation in the GitHub Copilot Application

***The GitHub Copilot application now deploys applications to the cloud using the open-source Radius project without requiring developers to write or maintain infrastructure as code.***

SAN FRANCISCO, CA—November 12, 2026—Today, GitHub and the Microsoft Azure Incubations team announced GitHub Radius, a set of new capabilities in the GitHub Copilot application for developers to define, deploy, and operate their cloud-native application entirely within the Copilot app. The Copilot app now integrates the open-source CNCF Sandbox project Radius to enable applications to be deployed to AWS, Azure, and Google Cloud with a simple chat command.

Modern application developers spend a disproportionate share of their time on deployment plumbing rather than on their applications. Even a simple web service plus a database typically requires the developer to pick an infrastructure as code (IaC) tool, learn a cloud provider's identity model, wire up a CI/CD workflow, and keep all of it working over time. Skills learned for one cloud rarely transfer to another. For individual developers, small teams, and open-source maintainers, this overhead is often the difference between shipping and abandoning a project.

GitHub Radius removes that overhead by making the application—not the infrastructure—the primary unit developers work with. The developer describes intent in natural language to GitHub Copilot. Copilot analyzes the developer’s repositories to identify the application’s services, dependencies, and runtime requirements. It then builds a cloud-agnostic deployment model and deploys the application to the developer's chosen cloud platform using the built-in deployment engine. Along the way, Copilot shows a visual representation of the application and its cloud resources. The same conversational workflow works whether the target is AWS, Azure, or Google Cloud, and whether the environment is dev, test, or production. 

“Developers use GitHub Copilot to build modern applications quickly. With GitHub Radius now in the Copilot app, they can deploy to the cloud just as easily using the same interface and AI agent they already use,” said Mario Rodriguez, Chief Product Officer at GitHub.

“AI can simplify cloud deployment, but it must do so without sacrificing determinism or control," said Mark Russinovich, Chief Technology Officer of Azure. "With GitHub Radius, we’re combining the accessibility of natural language with a system that produces predictable, inspectable outcomes developers can trust.”

GitHub Radius is delivered as a set of agent skills and dynamic visualizations built directly into the Copilot app, and an accompanying deployment engine running on GitHub Actions using a library of community-reviewed and maintained IaC modules. There is nothing to install on the developer's workstation beyond the Copilot app itself. When a developer asks Copilot to deploy an application, it presents a visualization of the application model the developer can review and refine. From that point forward, the entire application lifecycle is available through conversation: planning a deployment, deploying to an environment, promoting between environments, detecting and reconciling drift between the model and reality, modifying the application, and tearing it down cleanly when the work is done. For example, when a developer asks, “Deploy my Node.js application to AWS,” Copilot identifies dependencies (e.g., a container and a PostgreSQL database), constructs an application model, and deploys the application's container to Amazon Elastic Kubernetes Service (EKS) and Amazon Relational Database Service (RDS). If the developer asks, "Deploy to Azure" the same application would be deployed to Azure Kubernetes Service (AKS) and Azure Database for PostgreSQL.

GitHub Radius is available in public preview today for anyone with access to the Copilot app. To get started, open the Copilot app, add a repository, and ask Copilot to deploy the application. Learn more at https://radapp.io.

## Frequently Asked Questions

### General FAQs

**Q1: What is GitHub Radius?**

GitHub Radius is a capability inside the GitHub Copilot application that lets developers define, deploy, and operate cloud-native applications through conversation. It analyzes a repository to produce an application model, walks the developer through connecting a cloud environment, deploys the application on request, and can detect differences between the application deployment's desired and actual state. The developer only needs to interact with the Copilot app.

**Q2: What problem does GitHub Radius solve?**

Getting an application from a repository into the cloud is still the hardest part of most small projects. Developers must learn one or more IaC tools, a container runtime, the cloud provider's identity model, and the cloud provider's networking stack. They must build all of this themselves, keep it working, and rebuild most of it if they ever change clouds. The result is that a large fraction of a developer's time goes to deployment plumbing instead of to the application. AI coding agents can easily produce IaC, but the output can vary significantly between runs. A team that asks an AI to generate Terraform for the same application twice will get two different results, neither of which is guaranteed to be reviewed or production-ready.

GitHub Radius solves both of these challenges by splitting the problem in two: AI is used to model the application, while the infrastructure is provisioned using deterministic, community-reviewed and maintained IaC modules. The result is developers never have to worry about creating or maintaining IaC or other deployment scripts and artifacts.

**Q3: Who is GitHub Radius for?**

GitHub Radius is built for individual developers, business and data analysts building lightweight applications, and engineering teams at small businesses building line-of-business applications, prototypes, and internal tools. It is also for maintainers of open-source projects who want their contributors and evaluators to be able to deploy the project to their own cloud with a single conversation. GitHub Radius assumes the developer is comfortable with Git and GitHub but is not a Kubernetes operator or a cloud infrastructure expert.

GitHub Radius is not built for enterprise platform teams operating shared infrastructure for hundreds of developers. Teams with those requirements should continue to use the Kubernetes-hosted version of Radius.

**Q4: Why is GitHub Radius delivered inside the Copilot app?**

The Copilot app is the next generation of developer user experience. The terminal, the IDE, and the cloud console were each designed for a world in which the human typed every instruction. The Copilot app is designed for a world in which the human expresses intent and an AI agent does the work. GitHub Radius is built for that world. By living inside the Copilot app, GitHub Radius can be invoked the same way every other Copilot capability is invoked: the developer says what they want, and Copilot does it.

**Q5: What does "application-centric" mean here?**

Traditional cloud providers and IaC tools are infrastructure-centric. They manage individual cloud resources such as virtual machines, databases, and identity bindings, with no built-in understanding of which application those resources belong to. GitHub Radius is application-centric. The application is the unit of definition, deployment, and lifecycle. Every cloud resource Radius creates is owned by the application that created it, which makes promotion, drift detection, and clean teardown straightforward.

**Q6: How does GitHub Radius work end to end?**

When a developer uses Copilot to deploy an application, three steps are involved.

First, Copilot analyzes the repository. It looks at the source layout, Dockerfiles, framework hints, and declared dependencies, and produces a model of the application referred to as the *application graph*. This application graph includes abstract application resources that compose the application such as web services, workers, and databases, along with the relationships between them. Developers can explore this graph and the details of each resource in the Deployment panel in the Copilot app and ask Copilot to make changes as needed.

Second, before the application can be deployed, an environment must be defined. Before deploying, Copilot will help the developer define a deployment environment in AWS, Azure, or Google Cloud and configure authentication between Copilot and the cloud provider. Once the environment has been selected, Copilot will update the application graph on the Deployment panel with cloud provider-specific placeholder resources so the developer can examine exactly what cloud resources are planned. 

Finally, once confirmed by the developer, Copilot dispatches a workflow in GitHub Actions which starts the Radius deployment engine. The deployment engine reads the application graph, provisions the required resources in the developer's cloud account, and streams progress back to the Deployment panel in the Copilot app. Once completed, the application graph in the Deployment panel contains the full set of abstract application resources which constitute the model of the application, as well as the set of actual deployed cloud resources with real cloud resource identifiers. 

**Q7: How does GitHub Radius help me understand my application more?**

Visualization of applications and cloud environments is a key capability of GitHub Radius. At any time, a developer can open the Deployment panel in the Copilot application and view the application graph which contains the application resources. Right-clicking on an application resource shows a link to the source code that requires it.

If the application is deployed, the application graph also shows deployed cloud resources. Right-clicking on a cloud resource shows a deep link to the corresponding resource in Headlamp (if installed), the AWS console, Azure portal, or Google Cloud console.

As the application is modified, changes are highlighted on the application graph, enabling developers and PR reviewers to understand the scope of a change.

**Q8: What clouds does GitHub Radius support?**

GitHub Radius supports AWS, Azure, and Google Cloud. The same conversation works regardless of the target. The developer's existing cloud knowledge does not need to transfer between providers, because GitHub Radius handles the provider-specific translation.

> [!NOTE]
>
> **Internal**: Google Cloud support is not a MVP feature. See the fast-follow list in Q27.

**Q9: Can GitHub Radius deploy to my local development workstation or my on-premises environment?**

GitHub Radius is designed to deploy to AWS, Azure, or Google Cloud only. Enabling the local development loop is a future feature.

**Q10: What does the developer have to install?**

Only the Copilot app. There is no separate Radius CLI, no Dashboard, no local Kubernetes cluster, and no agent process to run on the workstation. The first time a developer asks Copilot to deploy an application in a given repository, Copilot offers to enable the deployment capability for that repository and installs the necessary agent skills and workflows on the developer's behalf with their consent. That is a one-time action per repository.

**Q11: How do I customize the application beyond what Copilot inferred?**

Just ask. Copilot understands the application as a graph of abstract resources, so the simplest way to make a change is to describe the change in conversation. Examples: "add a Redis cache to the web service," "use a custom domain on the API," or "swap postgres for a managed database in test and prod but keep it as a container in dev." Copilot updates the application graph and offers to redeploy the affected environments. 

**Q12: How do I define multiple environments such as dev, test, stage, and prod?** 

The first time Copilot is asked to deploy an application, it creates an environment after asking a series of questions including:

* the environment name,
* the AWS account ID and region or the Azure subscription ID and resource group,
* which container platform to use (Kubernetes, ECS, ACA, ACI, or Cloud Run) and which specific cluster to use,
* which OCI registry to use,
* and other infrastructure dependencies required by the application.

To create subsequent environments, simply ask Copilot to create a new environment.

> [!NOTE]
>
> **Internal**: ECS, ACA, and Cloud Run are not MVP features. See the fast-follow list in Q24.

**Q13: How are cloud credentials handled?**

GitHub Radius uses GitHub's native cloud federation. When a developer asks Copilot to create an environment, Copilot walks them through setting up a federated trust relationship between GitHub and their cloud account, stores the configuration as a GitHub environment, and validates that the federation works before continuing. No long-lived cloud credentials are ever copied into Copilot or stored on the workstation. The deployment workflow that runs in GitHub Actions exchanges a short-lived GitHub token for short-lived cloud credentials at deployment time.

**Q14: How does the deployment actually run? Is there a Radius control plane?**

There is no long-running control plane. Synchronous operations the developer expects to happen instantly, such as viewing the application graph or editing application resources, happen in the agent's process and complete in seconds. Asynchronous operations the developer already expects to take minutes, such as building images and provisioning cloud resources, happen in a GitHub Actions runner inside the developer's own repository. The Radius deployment engine runs inside the runner, performs the requested operation, writes status back to Radius data store, and exits. The developer's cloud credentials are stored as GitHub environment secrets and never leave GitHub.

**Q15: Is GitHub Radius idempotent? Does it detect drift?**

Yes. GitHub Radius compares the deployed application against the application graph and reconciles the two. The developer can ask Copilot at any time whether anything in the cloud differs from what they expect, and Copilot will report drift in natural language: resources that are missing, resources that have been modified outside the application graph, or resources present in the cloud that the application graph does not know about. Copilot offers two reconciliation paths for each case: update the application graph to match reality, or redeploy to make reality match the application graph.

> [!NOTE]
>
> **Internal**: Drift detection is not a MVP feature. See the fast-follow list in Q24.

**Q16: How does promotion between environments work?**

The developer asks Copilot to promote the application to the next environment by name, such as "promote to test." Copilot reuses the same application graph, layers in environment-specific configuration, shows the diff between what exists in the target environment today and what will exist after the promotion, and deploys on approval.

**Q17: What happens when I am done with a project?**

The developer asks Copilot to tear down a specific environment or every environment for the application. Copilot highlights on the Deployment panel exactly what will be destroyed, requires explicit confirmation, and dispatches a teardown workflow.

**Q18: How can I migrate from GitHub Radius to Kubernetes-based Radius?**

Migrating from GitHub Radius to Kubernetes-based Radius is easy. Both Radius options use the same set of resource types and recipes. After you install Radius on your Kubernetes cluster and configure your environments, ask Copilot to export your applications as Bicep files. You can take these `app.bicep` files and deploy them using `rad deploy`. It is not possible to migrate environments, credentials, or deployed applications from GitHub Radius to Kubernetes-based Radius.

> [!NOTE]
>
> **Internal**: Export to Bicep is not a MVP feature. See the fast-follow list in Q24.

**Q19: How does GitHub Radius compare to GitHub Spark?**

GitHub Spark and GitHub Radius solve different problems. Spark creates new micro apps from scratch through natural language conversation. The developer describes what they want, Spark generates the code and hosts the result on GitHub's infrastructure. The developer never sees a cloud account or a deployment step because Spark owns the full stack.

GitHub Radius starts where the developer already has code in a repository and wants to deploy it to their own cloud account. The developer keeps full ownership of the source code, the cloud resources, and the deployment architecture. Radius models the existing application, deploys it to AWS, Azure, or Google Cloud, and manages the lifecycle over time.

In short: Spark is for creating and hosting new lightweight apps on GitHub. Radius is for deploying existing applications from repositories to the developer's own cloud infrastructure. The two are complementary.

### Internal FAQs

**Q20: How is GitHub Radius different from the Kubernetes-based versions of Radius?**

Earlier versions of Radius were delivered as a Kubernetes-hosted control plane with a `rad` CLI as the primary client. That shape is the right fit for enterprise platform teams running shared infrastructure for many developers, and it remains the right option for those teams. GitHub Radius is a different delivery of the same core idea, optimized for individual developers, small teams, and open-source maintainers who are using the Copilot app. Key changes include:

* The Radius control plane (referred to as the Radius deployment engine above for customers) runs ephemerally in a GitHub Actions runner instead of permanently on Kubernetes.
* The GitHub Radius data store is stored within the Git repository and is not tightly bound to the Radius control plane. The exact implementtion of the storage including the file format is left to the technical design.
* Environments and environment metadata are stored as GitHub Environments and environment variables respectively rather than as Application.Core/environments or Radius.Core/environments resources.
* Deployments are stored as GitHub Deployments.
* The Radius CLI is replaced with Copilot and a set of agent skills and associated scripts which read and write from/to the Radius data store (for applications and resources) and GitHub (for environments and deployments).
* Resource groups are not used in GitHub Radius.
* Workspaces are not used in GitHub Radius.

**Q21: How does GitHub Radius relate to Repo Radius?**

Repo Radius was an earlier effort with the goal to ship a version of Radius for platform engineers to run within their continuous deployment pipeline, specifically GitHub Actions. It leveraged the Kubernetes-based Radius, adding capabilities to deploy to external Kubernetes clusters and store state in a Git repository rather than etcd.

GitHub Radius is an end-to-end solution for GitHub developers built around the Copilot app. It is the deployment feature of the Copilot app rather than an open-source distribution of the project. GitHub Radius includes the same technical capabilities as Repo Radius (external Kubernetes, Git storage), but has a different UX, is targeted to a different user persona, and is packaged and distributed differently.

We do not have concrete plans to ship Repo Radius as a standalone deployment option today. GitHub Radius is the priority.

**Q22: Are applications still being defined using Bicep?**

In GitHub Radius, developers no longer define applications themselves--Copilot does. Along with the overall storage design, the application definition file format will be defined in the technical design. 

**Q23: What new features does GitHub Radius introduce over the Kubernetes-based version?**

GitHub Radius introduces several new capabilities. The following constitutes the GitHub Radius minimum-viable product (MVP):

* **Application inference from source code.** Kubernetes-based Radius requires the developer to manually author the application definition in Bicep. GitHub Radius analyzes the repo (source layout, Dockerfiles, framework hints, dependencies) and models the application automatically. 
* **Deep linking from the application graph.** The application graph visualization in GitHub Radius includes the ability to right-click on a resource and click on a link to a relevant resource. For application resources, right-clicking shows a link to the source code which requires/composes the application resource (e.g. a Dockerfile for a container image). For cloud resources, right-clicking shows a link to the appropriate cloud console (e.g., a database links to the RDS database in the AWS console).
* **Preview deployments.** The ability to preview the cloud resources that will be created prior to an actual deployment. When previewing a deployment to a specific environment, the user is presented with an application graph which contains the abstract application resources (Radius.Compute/containers, Radius.Data/postgreSqlDatabases, etc.) as well as placeholder cloud resources (Kubernetes Deployment and Service, AWS RDS, Azure PostgreSQL Database Flexible Server, etc.).
* **Guided credentialing.** Kubernetes-based Radius assumes credentials are configured by a platform engineer using pre-existing credentials or with more advanced workload identity. GitHub Radius walks the developer through OIDC federation setup end-to-end without leaving the conversation.
* **Credentials are now tied to environments**. In Kubernetes-based Radius, there can be only a single credential for every AWS or Azure environment. Having only a single credential for production and non-production environments is a feature gap. In GitHub Radius, each environment has its own cloud provider credential.
* **Git-native graph storage.** Kubernetes-based Radius stores state in etcd on a Kubernetes cluster. GitHub Radius stores its applicaion graph external to the control plane within the Git repository.
* **Deployment to external EKS and AKS clusters**. Because GitHub Radius no longer runs as a perpetually-running Kubernetes-hosted control plane, GitHub Radius has the ability to deploy to an EKS or AKS cluster other than its host.

**Q24: Once GitHub Radius launches, what are the fast-follow features?**

After GitHub Radius launches, the following are fast-follow features:

* **Drift detection and reconciliation.** Kubernetes-based Radius lacks a "what's different between my application graph and what's actually deployed?" capability. GitHub Radius adds explicit drift detection with two reconciliation paths (update application graph to match reality, or redeploy to match application graph).
* **Export to Bicep**. To support a migration from GitHub Radius to Kubernetes-based Radius, GitHub Radius will have the ability to export an `app.bicep` file for one or all applications stored in GitHub Radius.
* **Google Cloud support**. As with Kubernetes-based Radius, support for Google Cloud is planned but is not considered a MVP requirement.
* **Additional serverless platform support.** In addition to supporting Kubernetes and ACI today, GitHub Radius will support ECS, ACA, and Cloud Run (in this order) in the future.

**Q25: What about the existing resource types and recipes in the `resource-types-contrib` repo?**

GitHub Radius continues to use resource types and recipes as the mechanism by which abstract application resources become concrete cloud resources. The Radius `resource-types-contrib` recipe library is reused, now with greater importance. What changes is the expectation of how mature and comprehensive the resource types and recipes must be.

In Kubernetes-based Radius, the enterprise's platform engineer was expected to either bring their own recipes or substantially customize Radius' recipes. In GitHub Radius, the Radius project is now responsible for ensuring the resource types and recipes are production ready and follow security and cost best practices. Because of this shift, the Radius project will need to invest more resources in the development, testing, and maturing of the existing recipe library.

**Q26: How does the developer know Radius is involved?**

By design, they do not, unless they look. The Copilot app is the surface. The conversation is the interface. The application graph is stored in their repository. If a developer wants to understand how the capability works or where to ask for help, a discreet "powered by Radius" reference and an `/about` command in the skills point at the Radius project and community. We expect the vast majority of users will never need either.

**Q27: What is the relationship between GitHub Radius and the Kubernetes-based version going forward?** 

Our objective remains the broad adoption of Kubernetes-based Radius by enterprises. GitHub Radius is targeted at a distinctly different non-enterprise persona. We believe that, with Radius built into the Copilot app, enterprises will gain greater exposure to Radius, which will drive adoption of Kubernetes-based Radius. 

Developers who use GitHub Radius for side projects and prototypes will encounter Radius's application graph, and when they move to an enterprise team with shared infrastructure needs, they will already understand the mental model and advocate for Kubernetes-based Radius.

**Q28: What new GitHub Radius features will be implemented in Kubernetes-based Radius?**

Our primary objective is building GitHub Radius. However, in several situations it is appropriate to generalize new GitHub Radius features and deliver them as Radius features in both GitHub Radius and Kubernetes-based Radius. These generalizable features include:

* Deployment to external EKS and AKS clusters
* Drift detection and reconciliation

Kubernetes-based Radius will also benefit from the larger number of resource types and more mature recipes in the `resource-types-contrib` repository.

**Q29: What new GitHub Radius features are not generalizable but could be implemented in Kubernetes-based Radius?**

Many other new GitHub Radius features could be implemented in Kubernetes-based Radius. However, they are not generalizable enough across the two solutions. 

* Deep linking from the application graph can be implemented in the Radius Dashboard and the Radius plug-in for Headlamp.
* Preview deployments can be implemented in `rad deploy`.
* Guided credentialing can be implemented in `rad init`.

These features will be considered as capacity allows, but the primary investment is in GitHub Radius.

