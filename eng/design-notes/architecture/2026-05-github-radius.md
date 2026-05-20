# GitHub Radius: Application Graph Challenges

**Author**: Zach Casper (@zachcasper)

This is a short document in response to the [Application Graph Visualization design](https://github.com/radius-project/radius/pull/11712/). 

GitHub Radius is a version of Radius that is tightly integrated with GitHub. Rather than Radius running as a long-running control plane on a Kubernetes cluster and a light-weight CLI, GitHub Radius runs as a Radius agent with a set of skills within GitHub Copilot and a version of Radius optimized for running in a GitHub Actions runner. The implications of these changes are significant:

* All synchronous user operations must occur outside of the Radius control plane. Any operation that requires the Radius control plane will always take multiple minutes. That means Radius operations such as listing applications and resources, or viewing the modeled or planned application graph must happen outside the Radius control plane. 
* Asynchronous operations such as deployments can continue to run via the Radius control plane.
* There is no Radius CLI. The only client is GitHub Copilot in the near term and the GitHub.com web application in the distant future.

## Proposed Execution Model

Given the need to have synchronous operations occur outside of the control plane, the fundamental architecture of Radius is changed from a centralized control plane with a JSON-based CRUDL API where all data is read or written via the API so a multi-client architecture centered on a common data store. Concretely:

* The Radius data store is a JSON-based graph database stored in a Git orphaned branch in the Git repository. Add Radius data is mastered in this data store. Any data outside of this data store is not authoritative. There may need to be a locking mechanism.
* All synchronous user operations are implemented as a Radius agent with a set of skills. The Radius skills reads and writes directly to the Radius data store using scripts. These skills use a set of scripts centrally maintained by the Radius project and implement the Radius API to CRUDL resources from the Radius data store.
* Deployment operations continue to be executed by the Radius control plane asynchronously. Rather than writing to etcd or PostgreSQL, the Radius control plane writes directly to the Radius data store. As deployment status changes in realtime, Radius maintains the status in the Radius data store to enable realtime visualization for the user.

## Proposed Data Model

The application graph is a critical component of GitHub Radius. In the fullness of time, we expect the application graph to be rich with as many resources as possible. This includes:

* Application resources such as containers, container images, databases, and message queues 
* Cloud resources that materialize the application resources such as Kubernetes deployments, RDS databases, Azure Event Hubs
* Source code that compose the application resources such as Dockerfiles
* Supporting cloud resources such as Kubernetes clusters, OCI registries, load balancers, certificates, etc.

GitHub Radius will has three different application graph views:

1. **Application model**. This graph view is the output of the source code analysis and includes:
   - Application X
     - Abstract application resource such as Radius.Compute/containers and Radius.Data/postgreSqlDatabases
       - Source code that compose these resources
2. **Planned application graph**. This graph view is a what-if view of what the graph will look like after deploying the application to a specific environment. It includes:
   - Environment X
     - AWS account and region or Azure subscription and resource group
       - Cloud resources that materialize each application resources (but with synthetic IDs)
   - Application X
     - Abstract application resource such as Radius.Compute/containers and Radius.Data/postgreSqlDatabases
       - Source code that compose these resources
3. **Deployed application graph**. This graph view includes:
   -  Environment X
      -  AWS account and region or Azure subscription and resource group
         -  Supporting cloud resources such as Kubernetes clusters and OCI registries
            -  Cloud resources that materialize each application resources (with resource IDs)
      -  Application X
         -  Abstract application resource such as Radius.Compute/containers and Radius.Data/postgreSqlDatabases
            -  Source code that compose these resources

Notice that the data model is a single graph of application definitions including its application resources and environments with the cloud resources. Therefore, the Radius data model is a unified data store. 

## User Journey

The user journey would be similar to:

1. The developer invokes the Radius agent using GitHub Copilot and asks it to deploy an application in the Git repository. The Radius agent responds that it must first create a model of the application's deployment architecture.
   - The Radius agent analyzes the Git repository and identifies required cloud resources and creates an application in the Radius data store (the `.radius` orphaned branch). 
   - It does not create an app.bicep file because (1) there would be duplicate information stored in the Git repository and (2) the Radius data store is the master for the application definition and the Radius data store is JSON based. 
     - In the future, GitHub Radius will have an export to Bicep capability that allows users to edit the app.bicep file directly. This, however, is not a MVP capability.
   - The developer is shown the application model by visualizing the JSON files stored in the Radius data store.
2. The developer can interact with the Radius agent to modify the application model.
   -  The Radius agent updates the Radius data store directly based on the developer's input.
3. The Radius agent asks the developer if they want to setup an AWS or Azure environment to deploy the application to.
   - The Radius agent creates an environment in the application graph. 
   - The developer is taken through the journey of connecting the new environment with their cloud provider.
4. The Radius agent asks the developer if they want to deploy the application.
   -  The developer says yes. 
   -  The Radius agent uses its skill to add placeholder cloud resources to the application graph:
      -  It calls the GitHub API to read the location of recipes for the target environment. 
      -  For each resource in the application model, it examines the recipe and identifies the cloud resources which will be created.
      -  It then writes placeholder cloud resources within the environment in the application graph.
   -  The Radius agent opens a side bar that shows the planned application graph view.
   -  The Radius agent uses its skill to call the GitHub API to dispatch a deployment workflow.
   -  The workflow starts the Radius control plane in a GitHub Actions runner and the deployment operation is started.
   -  As the workflow progresses, the Radius control plane updates the placeholder cloud resources with actual cloud resources by updating each resource with an actual cloud provider ID.
5. The developer watches realtime updates to the application graph as the deployment proceeds.
