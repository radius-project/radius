# Radius Integration with GitOps

* **Author**: Will Tsai (@willtsai)

## Summary
<!-- A paragraph or two to summarize the Epic. Just define it in summary form so we all know what it is. -->
Continuous deployment of cloud native applications and infrastructure is challenging for a number of reasons.  GitOps is a popular set of practices, implemented as popular tools, like Flux and ArgoCD, that mitigates these challenges for enterprise application teams that use git for source control and Kubernetes for orchestration of software containers.  GitOps provides a developer-centric experience by abstracting and automating many of the tasks required for deploying and operating Kubernetes and its underlying dependent infrastructure. The core concept of GitOps is to rely on a git repository that serves as a single source of truth: i.e. it contains current declarative descriptions of the required infrastructure for a given production environment. It also contains a description of the workflow required to prevent drift between the repo and the production environment. It's like having cruise control for managing applications in production without needing to write custom scripts or maintain complex CD pipelines.

Enterprises that use GitOps and also want to use Radius do not currently have a clear path for how to use both technologies in an integrated and complementary way to enable a “better together” experience.

>It's important to note that GitOps mechanisms don't manage the CI (e.g. image build and app containerization) but they do manage the CD (e.g. deployment of the app to the cluster)

### Top level goals
<!-- At the most basic level, what are we trying to accomplish? -->
- Enable enterprises to get the best of both GitOps and Radius for a better-together experience.
- Provide a consistent Radius + GitOps integration model for both existing and future GitOps platforms.

### Non-goals (out of scope)
<!-- What are we explicitly not trying to accomplish? -->
- We are not trying to replace GitOps tools or platforms.  We are trying to integrate Radius with existing GitOps tools and platforms.
- We are not trying to replace GitOps practices.  We are trying to enable Radius to work well with GitOps practices.
- We are not implementing dashboard visualizations for GitOps changes (i.e. as an Operator and as a Developer, I can visualize the changes tracked by my GitOps toolset on my Radius Dashboard) as a part of the initial scope of this project. We still need to flesh this out but the basic idea is the Radius dashboard can leverage the Notification Controller in Flux and ArgoCD to display to users inbound and outbound information of the source changes and cluster changes affecting their application and resources. In a future iteration of Radius GitOps integration, we will explore options and potential to leverage plugins in the Backstage ecosystem to provide this functionality.

## user profile and challenges
<!-- Define the primary user and the key problem / pain point we intend to address for that user. If there are multiple users or primary and secondary users, call them out.   -->

### User persona(s)
<!-- Who is the target user? Include size/org-structure/decision makers where applicable. -->
**Infrastructure operators / administrators** - In enterprise applications team that use GitOps, operators are responsible for configuring and managing the git repos for infrastructure and application configuration. They configure GitOps policies as well as the Kubernetes manifests, Helm charts, etc. that are applied to the application and the Kubernetes cluster.

**Application developers** - Responsible for designing, developing and maintaining application code. Developers use GitOps by checking files into their git repositories to configure GitOps settings to auto deploy and monitor their application and dependent infrastructure.

**Site Reliability Engineers** - They leverage GitOps by checking in configuration files into their git repositories that add information for the customization of the workload, e.g. replica pod number. 

### Challenge(s) faced by the user
<!-- What challenges do the user face? Why are they experiencing pain and why do current offerings not meet their need? -->
Before the introduction of GitOps, enterprise application teams struggled with complex deployment processes; environment/application configuration drift; as well as difficulty following best practices for security and disaster recovery.  GitOps enables teams to define continuous deployment workflows and automation without having to write custom scripts or maintain complex CI/CD pipelines. GitOps tools simplify and perform many of these tasks, which add significant value by increasing efficiency, security, creating better developer experiences, and allowing for faster deployments between code changes.

Radius offers additional complementary value for enterprise application teams, including a rich, declarative application description which enables the Radius application graph.  Radius also provides a clear separate of concerns via Recipes.  However, without integration of Radius and GitOps, plus clear guidance on how to use the two together, enterprises already invested in GitOps don't know how to use both GitOps and Radius for a better together experience.

Additionally, in its current state GitOps works well within the Kubernetes sphere of influence but can't be applied to things outside of Kubernetes.  This is a limitation for enterprises that want to use GitOps to manage their entire application stack, including things like databases, storage, and other infrastructure.  To get the full benefit of GitOps, a user needs to combine things that can be controlled with GitOps with things that can't. Radius + Recipes can help here.

<!-- What is the positive outcome for the user if we deliver these features, i.e. what is the value proposition for the user? Remember, this is user-centric. -->

## Key scenarios
<!-- List ~3-7 high level scenarios to clarify the value of the Epic and point to how we will decompose this big area into component capabilities. We may ultimately have more than one level of scenario. -->

### Scenario 1: As an operator I can install and configure Radius with my GitOps tool of choice
<!-- One or two sentence summary -->
My team uses GitOps to manage Kubernetes clusters and applications that run in those clusters.  All relevant configuration information is stored in git repositories that serve as the sources of truth for both the application code and infrastructure configuration.  To use Radius, I need to include Radius as a dependency for my Kubernetes cluster, just as I would any other dependency, via my GitOps tools.

### Scenario 2: As an operator or developer, I can deploy and manage Radius Environments, Recipes, applications, resources, credentials, etc. via my GitOps toolset
<!-- One or two sentence summary -->
With Radius installed, as an Operator, I now define, deploy and manage a Radius environment and associate relevant Recipes, Credentials, and other Radius objects to that environment. To accomplish this, I just need to push my Radius Environment, Recipe, and other files into my git repository. Flux or ArgoCD will detect the changes and trigger Radius to deploy the Environment as specified.

As a Developer, my Operations counterpart has provided a Radius Environment plus Recipes to enable my Radius application development. After I have familiarized myself with Radius, I push my application definition files into my git repository, where Flux or ArgoCD will detect the changes and trigger Radius to deploy the application to this environment using the Recipes. After my app is deployed, I can view its graph in the Radius dashboard. Everything is working well for me so far and I really appreciate these cool Radius features!

### Scenario 3: As a Site Reliability Engineer (SRE), I can edit and customize the configurations in the Kubernetes cluster through my GitOps toolset, even after the application has been previously deployed.
<!-- One or two sentence summary -->
As an SRE, my Operations counterpart has provided a Kubernetes cluster, a Radius Environment running on that cluster and the infrastructure resources through Radius Recipes required for the Radius Applications that will run in that Radius Environment. I am expected to make adjustments to the configuration of the Kubernetes cluster and the Radius Environment as needed. I can do this through my GitOps toolset by updating the Bicep parameter files and pushing them into the git repository. Flux or ArgoCD will detect the changes and trigger Radius to redeploy the resources with the new configurations.

>Note that this GitOps integration with Radius will not handle the publishing step of recipes, i.e. `rad bicep publish`. Instead, changes to the environment (e.g. Recipe image tag) would be built into the definition of `env.bicep` and thus applied by GitOps as the Recipe configurations in the Environment are updated and merged.

## Key dependencies and risks
<!-- What dependencies must we take in order to enable this scenario? -->
<!-- What other risks are you aware of that need to be mitigated. If you have a mitigation in mind, summarize here. -->
<!-- **Dependency Name** – summary of dependency.  Issues/concerns/risks with this dependency -->
<!-- **Risk Name** – summary of risk.  Mitigation plan if known. If it is not yet known, no problem. -->
Dependencies - Flux, ArgoCD, future GitOps tools/platforms. 

The primary risk, per the question below, is whether an abstracted, generalizable extensibility model for GitOps is feasible. Preliminary investigation tells us that it is feasible via a Kubernetes controller.

## Key assumptions to test and questions to answer
<!-- If you are making assumptions that, if incorrect, would cause us to significantly alter our approach to this scenario, make them explicit here.  Also call out how / when you plan to validate key assumptions. -->
<!-- What big questions must we answer in order to clarify our plan for this scenario.  When and how do you plan to answer those questions (prototype feature x, user research, competitive research, etc) -->
Our goal is to deliver a generalized model such that Flux, ArgoCD plus future GitOps technologies can all be integrated in a consistent manner.  How feasible is that?  We'll need to learn more about the GitOps tool landscape to determine feasibility.

We as the Radius team understand that while Flux and ArgoCD might currently be the most popular options utilized by users and while the current scenarios might reference Flux or ArgoCD, the solution we will build will be abstract enough to support all other high priority GitOps toolsets. We currently understand the GitOps workflow if abstracted in implementation usually follows this format:
1. Install GitOps tool onto Kubernetes cluster. 
2. Add git repository information to GitOps tool
3. Deploy application to Kubernetes cluster 
4. Done 
 
> The implementation settled on is generic enough to support GitOps on a wide level not narrowed down to just Flux or ArgoCD.

## Current state
N/A

# Scenario 1: Install and configure Radius using GitOps

## Target users
**Infrastructure operators / administrators** - In enterprise applications teams that use GitOps, operators are responsible for configuring and managing the git repos for infrastructure and application configuration. They configure GitOps policies as well as the Kubernetes manifests, Helm charts, etc. that are applied to the application and the Kubernetes cluster.

## Details of user problem
As an infrastructure operator in an enterprise applications team that has to maintain complex Kubernetes clusters and the platforms that run on them, I need to ensure that my team can deploy and manage applications and infrastructure in a consistent and reliable way. Thus, we make use of GitOps to manage Kubernetes clusters and applications that run in those clusters. All relevant configuration information is stored in git repositories that serve as the source of truth for both the application code and infrastructure configuration. Most commonly, infrastructure configuration and application code are housed in separate repos. To use Radius, I need to include Radius as a dependency for my Kubernetes cluster, just as I would any other dependency, via my GitOps tools. Without clear guidance on how to do this, I am unsure how to use both GitOps and Radius for a better together experience.

## Desired user experience outcome
<!-- <Write this as an “I statement” that expresses the new capability from user perspective … i.e. After this scenario is implemented “I can do, x, y, z, steps in cloud native app developer and seamlessly blah blah blah …. As a result <summarize positive impact on your work / business>  -->
After this scenario is implemented, I can install and configure Radius with my GitOps tool of choice. I can include Radius as a dependency for my Kubernetes cluster, just as I would any other dependency, via my GitOps tools. This ensures that I no longer have to manually manage the Radius instance I am providing for my developers and SREs to use, that there is an automated process to ensure that the deployed Radius instance matches the state specified I have specified in a repository. In other words, my experience with managing our Radius instance is consistent with how I normally use GitOps to define any other dependency in a Kubernetes environment.

### Detailed User Experience
 <!-- <List of steps the user goes through from the start to the end of the scenario to provide more detailed view of exactly what the user is able to do given the new capabilities>  -->
<!-- Step 1
Step 2
… -->
1. With my editor of choice (such as VSCode) open the GitOps (Flux or ArgoCD) Source File  
1. Add installation of Radius as a Kubernetes cluster dependency for my desired Radius version (committing a new version number in the repo triggers an upgrade of the Radius control plane).
   1. Similar the params in the Flux schema defined here: [Helm Charts | Flux (fluxcd.io)](https://fluxcd.io/flux/components/source/helmcharts/)
1. Commit the change to the git repo to submit my change and track it for future reference.
1. GitOps tool will detect this repo change and update the relevant Kubernetes clusters 
1. I can validate the Radius installation through various GitOps or Kubernetes tools. For example, I can run `kubectl get namespace` and verify Radius appears as Kubernetes namespace. 
   1. I will be able to validate Radius on my Flux dashboard or my rad CLI/GitOps CLI as well once I've created a Radius Environment or have Radius Applications running.

> As an operator, I'm happy with this experience because it is completely consistent with how I normally use Flux or ArgoCD to define any other dependency in a Kubernetes environment.
 
The syntax for adding Radius as a Kubernetes cluster dependency must be:
- Consistent with user expectations for defining such a dependency 
- Executable by Flux or ArgoCD (which based on current understanding should just work today). 

> There is no new Radius specific user experience for this scenario. Users are only interacting with Flux, ArgoCD, or their GitOps tool of choice.

## Key investments
<!-- List the features required to enable this scenario. -->
Based on current understanding, there should be no new features that need to be implemented in Radius for this scenario. The key investment is in testing the GitOps tooling to ensure that the syntax for adding Radius as a Kubernetes cluster dependency is consistent with user expectations for defining such a dependency and is executable by Flux or ArgoCD. We need to validate that our Helm Charts are compatible with Flux and ArgoCD.

## Key dependencies and risks
<!-- What dependencies must we take in order to enable this scenario? -->
<!-- What other risks are you aware of that need to be mitigated. If you have a mitigation in mind, summarize here. -->
<!-- Dependency Name – summary of dependency.  Issues/concerns/risks with this dependency -->
<!-- Risk Name – summary of risk.  Mitigation plan if known. If it is not yet known, no problem. -->

**Dependency: Flux, ArgoCD, etc.** - These GitOps tools/platforms must be able to detect the Radius dependency specified in the GitOps source file and update the relevant Kubernetes clusters accordingly to install Radius.

**Risk: ability for GitOps tools to install and deploy Radius** - The primary risk is whether Flux, ArgoCD, etc. can install and deploy Radius as a Kubernetes cluster dependency. We will need to test this to ensure that it works as expected by validating that Radius Helm Charts are compatible with Flux and ArgoCD.

## Key assumptions to test and questions to answer
<!-- If you are making assumptions that, if incorrect, would cause us to significantly alter our approach to this scenario, make them explicit here.  Also call out how / when you plan to validate key assumptions. -->
<!-- What big questions must we answer in order to clarify our plan for this scenario.  When and how do you plan to answer those questions (prototype feature x, user research, competitive research, etc) -->

**Assumption:** The syntax for adding Radius as a Kubernetes cluster dependency must be consistent with user expectations for defining such a dependency and must be executable by Flux or ArgoCD. We will need to test this to ensure that it works as expected.

# Scenario 2: Deploy and manage Radius Environments, Recipes, Apps, Resources using GitOps

## Target users
<!-- Of the users / personas listed in the Epic doc, what subset are we delivering this scenario to serve? -->
**Infrastructure operators / administrators** - In enterprise applications team that use GitOps, operators are responsible for configuring and managing the git repos for infrastructure and application configuration. They configure GitOps policies as well as the Kubernetes manifests, Helm charts, etc. that are applied to the application and the Kubernetes cluster.

**Application developers** - Responsible for designing, developing and maintaining application code. Developers use GitOps by checking files into their git repositories to configure GitOps settings to auto deploy and monitor their application and dependent infrastructure.

## Existing user problem
<!-- <Write this in first person. You basically want to summarize what “I” as a user am trying to accomplish, why the current experience is a problem and the impact it has on me, my team, my work and or biz, etc…. i.e. “When I try to do x aspect of cloud native app development, I have the following challenges / issues….<details>. Those issues result in <negative impact those challenges / issues have on your work and or business.> -->
Since my organization uses GitOps to manage Kubernetes clusters and applications that run in those clusters, I need to remain consistent in infrastructure and application deployment even when using Radius. This means that I need to be able to apply Radius-centric concepts like Environments, Recipes, Applications, and Resources in a way that is consistent with my existing GitOps workflows. Without clear guidance on how to do this, I am unsure how to use both GitOps and Radius for a better together experience.

## Desired user experience outcome
<!-- <Write this as an “I statement” that expresses the new capability from user perspective … i.e. After this scenario is implemented “I can do, x, y, z, steps in cloud native app developer and seamlessly blah blah blah …. As a result <summarize positive impact on your work / business>  -->
With the implementation of this scenario, I can deploy and manage Radius Environments, their associated Recipes, Applications, Credentials, and Resources via my existing GitOps (Flux and ArgoCD) toolsets. This means that I don't have to author pipelines or manually run Radius commands to deploy Radius-managed resources. Instead, I can leverage a git repo as the source of truth for my Radius-managed resources and use my existing GitOps tooling to deploy and manage those resources. This ensures that my experience with managing Radius resources is consistent with the patterns in how I normally use GitOps to manage Kubernetes clusters and applications.

> There is no new Radius specific user experience for this scenario. Users are only interacting with Flux or ArgoCD. The new experience overall is using Radius and Bicep with GitOps instead of YAML/KRM.

### Detailed user Experience
 <!-- <List of steps the user goes through from the start to the end of the scenario to provide more detailed view of exactly what the user is able to do given the new capabilities>  -->
<!-- Step 1
Step 2
… -->

**Step 1: Define and deploy Radius Environment and Recipes**

As an operator, I need to define and deploy a Radius Environment and associate relevant Recipes, Credentials, etc. to that environment so that my development team can build and deploy their Radius applications.

1. Using my editor of choice, such as VSCode, I open an existing Radius environment file, `env.bicep`, from the online Radius samples repo. 
1. I use Radius documentation to determine specific edits to my `env.bicep` and `env.bicepparams` files such as Kubernetes cluster and the appropriate cloud provider registrations and credentials. 
   1. I will create a Radius Environment resource and add all the information listed above. 
   1. I will then edit my Radius Environment resource to contain the properties required to register my Radius Recipe such as template path, template kind, etc.
1. I then push my new `env.bicep` and `env.bicepparams` files to my git repository, as I would any other file. 
1. This triggers Flux or ArgoCD to detect the change, read the new `env.bicep` and `env.bicepparams` files, and trigger Radius to deploy the Environment as specified.
1. After Flux or ArgoCD initiates the Radius environment deployment, as I would normally do, I use Flux or ArgoCD to monitor deployment status and health status of the underlying Kubernetes cluster. 

> As an operator, while Radius presents some new concepts, those concepts fit cleanly within my existing GitOps workflows so Radius is relatively low overhead for me to adopt and to provide value to my development counterparts.

> A similar process is followed for deploying Radius Recipes, Applications, or any other Resources.

**Step 2: Deploy and manage Radius Applications and Resources through GitOps toolset**

Now, as a developer, I need to make some changes to my application code. Specifically, I need to change the Radius Resource for my frontend UI. My frontend resource is already running in my Radius application but I need update the code for parsing user input. First, I'll update the required file where this function lives then I'll update my Radius Resource as follows: 

1. With my editor of choice, such as VSCode, I open the existing `frontend` application file `helper.ts` and update my function `parseUsers` with the new required logic. 
2. Then I push the changes to my git repository, which triggers my CI pipeline to rebuild and containerize my application. *Note that the CI is outside of the scope of GitOps and this scenario.*
3. This triggers Flux/ArgoCD to detect the updated application image and update the `frontend` resource with the new function. I use ArgoCD to monitor the health status of the Kubernetes cluster and immediately realize the pod associated with my `frontend` resource is failing.
4. I review my code change again and realize I created a bug in the function which is causing the resource to fail. 
5. To resolve, I:
   1. Use the ArgoCD [rollback commands](https://argo-cd.readthedocs.io/en/release-2.7/user-guide/commands/argocd_app_rollback/) to restart my `frontend` resource pods with the previous code version so as not to disrupt my users OR I use `git revert` to revert my code change and `git push` the revert to my git repository which then gets detected by my CI to rebuild the image and then gets redeployed by Flux.
   1. Proceed with a new code change to my `helper.ts` that resolves the bug, then push this change to my repository. 
6. Flux/ArgoCD now triggers a redeployment of my new application image containing the bug fix and I can see in Flux/ArgoCD that the pod has restarted with my new code, but my application is still failing due to connectivity issues with a dependency of the `frontend` service that was added as a result of my code changes above. I realize that I need to update the `frontend` resource to include a new environment variable that points to the new dependency.
7. In VSCode, I open the `frontend.bicep` Radius definition and `frontend.bicepparams` files and add the new environment variable to the `frontend` resource, then push the change to my git repository.
8. Flux/ArgoCD detects the change and triggers Radius to redeploy the `frontend` resource with the updated environment variable. I can now see in Flux/ArgoCD that the pod has restarted and is now healthy.

> As a Developer, I'm very happy with this experience because I get the cool new features of Radius (like self-serve infrastructure deployment via Recipes and the App Graph/Dashboard) in addition to the features I already know and love in Flux, like health monitoring and rollback.

__Requirements resulting from these scenarios:__ 
- Flux/ArgoCD must be able to detect the file change described above and must be able to read the env.bicep file as required to deploy the Radius app correctly 
- Flux/ArgoCD must be able to detect changes to the `.bicepconfig` file and execute bicep config updates
- Radius types, `bicep`, and `bicepparams` files must be able to be read by Flux/ArgoCD which currently does not have that ability.
- Flux/ArgoCD must be able to detect Radius resource types and execute Flux/ArgoCD commands against those types, including rollback *list of any other required Flux commands*.
- Based on user feedback this could be just testing that commands such as dry run mechanisms work

## Key investments
<!-- List the features required to enable this scenario. -->

### Feature 1: GitOps tooling can detect and understand Radius definition files
<!-- One or two sentence summary -->
GitOps tools (beginning with Flux and ArgoCD) must be able to detect the file change in the git repo described above and must be able to read the `env.bicep` file (or any `*.bicep`, `*.bicepparams`, `.bicepconfig` files) as required to deploy the Radius app correctly.

GitOps tools (beginning with Flux and ArgoCD) must be able to detect changes to Radius applications and other resource type definitions in the git repo and notify Radius of the changes.

### Feature 2: GitOps tooling can trigger Radius to execute commands
<!-- One or two sentence summary -->
Once Radius definition file changes are detected and understood, GitOps tools (beginning with Flux and ArgoCD) must be able to notify and trigger Radius to execute deployments for the Radius resources defined in the git repo.

### Feature 3: GitOps tooling can rollback Radius resources
<!-- One or two sentence summary -->
GitOps tools (beginning with Flux and ArgoCD) must be able to execute rollback operations for Radius resources defined in the git repo. In other words, the user may use the GitOps rollback commands or `git revert` to revert a change to a Radius resource and Flux/ArgoCD will detect the change and trigger Radius to redeploy the resource with the previous configuration.

### Feature 4: GitOps tooling can detect and execute resource deletions
<!-- One or two sentence summary -->
If a resource or config in a `.bicep`, `.bicepparams`, or `.bicepconfig` definition file is deleted and the change is pushed to the repo, the GitOps tools (Flux and ArgoCD) should detect the change and trigger Radius to delete that resource. Note that this is not a behavior of Bicep today but it's important for this usecase.

>The technical design should account for the edge case where `.bicepparams` files are deleted but the `.bicep` files are not. The behavior for this case should follow the same conventions currently in place for Helm charts and other GitOps resources - more investigation during implementation time is needed to determine the behavior that aligns with conventions. Reference the concept of "declarative deletion" in the exploration.

## Key dependencies and risks
<!-- What dependencies must we take in order to enable this scenario? -->
<!-- What other risks are you aware of that need to be mitigated. If you have a mitigation in mind, summarize here. -->
<!-- Dependency Name – summary of dependency.  Issues/concerns/risks with this dependency -->
<!-- Risk Name – summary of risk.  Mitigation plan if known. If it is not yet known, no problem. -->

**Dependency: Flux, ArgoCD, etc.** - These GitOps tools/platforms must be able to detect the Radius definition files specified in the git repo and update the relevant Kubernetes clusters accordingly to deploy and manage Radius resources.

**Risk: ability for GitOps tools to deploy and manage Radius resources** - The primary risk is whether Flux, ArgoCD, etc. can trigger Radius to deploy and manage resources as specified in the git repo. We will need to test this to ensure that it works as expected.

## Key assumptions to test and questions to answer
<!-- If you are making assumptions that, if incorrect, would cause us to significantly alter our approach to this scenario, make them explicit here.  Also call out how / when you plan to validate key assumptions. -->
<!-- What big questions must we answer in order to clarify our plan for this scenario.  When and how do you plan to answer those questions (prototype feature x, user research, competitive research, etc) -->
**Assumption:** GitOps tools (beginning with Flux and ArgoCD) must be able to detect the file change in the git repo described above and must be able to read the Bicep files as required to trigger Radius to deploy the resources correctly. We will need to test this to ensure that it works as expected.

**Assumption:** CI for the application code and Recipes is outside of the scope of this scenario. We assume that the user has separate automation that handles the containerization of the application code and the publishing of the Recipes to a registry.

# Scenario 3: Patch and edit the configuration in my Kubernetes cluster through my GitOps toolset

## Target users
<!-- Of the users / personas listed in the Epic doc, what subset are we delivering this scenario to serve? -->
**Site Reliability Engineers (SREs)** - They leverage GitOps by checking in configuration files into their git repositories that add information for the customization of the workload, i.e. replica pod number, strategic merge solutions.

## Existing user problem
<!-- <Write this in first person. You basically want to summarize what “I” as a user am trying to accomplish, why the current experience is a problem and the impact it has on me, my team, my work and or biz, etc…. i.e. “When I try to do x aspect of cloud native app development, I have the following challenges / issues….<details>. Those issues result in <negative impact those challenges / issues have on your work and or business.> -->
As an SRE, I am expected to make adjustments to the configuration of the Kubernetes cluster and the Radius Environment as needed. Today, I adjust the cluster as needed through my GitOps toolset via patching solutions like [Kustomize](https://kubernetes.io/docs/tasks/manage-kubernetes-objects/kustomization/) in [Flux](https://fluxcd.io/flux/components/kustomize/) or [ArgoCD](https://argo-cd.readthedocs.io/en/stable/user-guide/kustomize/), but I'm unable to do so for all the Radius-specific resources since Radius types and Bicep files are not currently readable by Flux or ArgoCD. This means that I have to manually manage the Radius instance I am providing for my developers and operators to use, which is a significant overhead for me to adopt and to provide value to my development counterparts. Additionally, access to modify the Radius Applications and Resources is not granted to me as an SRE, so I have to rely on my Operations counterpart to make these changes for me, which is a significant bottleneck in my workflow.

## Desired user experience outcome
<!-- <Write this as an “I statement” that expresses the new capability from user perspective … i.e. After this scenario is implemented “I can do, x, y, z, steps in cloud native app developer and seamlessly blah blah blah …. As a result <summarize positive impact on your work / business>  -->
As an example, the application team will require 10 replica pods for their prod environment however this variable is currently set to 1 in the current [Bicep parameters](https://learn.microsoft.com/en-us/azure/azure-resource-manager/bicep/parameter-files?tabs=Bicep) file created by the operator or developer called `app.bicepparams` in the parameters git repo:

````bicep
using './app.bicep'

param replicaCount = 1
````

The above parameter is defined and used in the `app.bicep` file stored in a separate repo as follows:

```bicep
import radius as radius

@description('The app ID of your Radius Application. Set automatically by the rad CLI.')
param application string
param replicaCount int

resource demo 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'demo'
  properties: {
    application: application
    container: {
      image: 'ghcr.io/radius-project/samples/demo:latest'
      ports: {
        web: {
          containerPort: 3000
        }
      }
      extensions: [
      {
        kind:  'manualScaling'
        replicas: replicaCount
      }
    ]
    }
  }
}
```

To fix this as a SRE, I can:

1. Open the `app.bicepparams` file in my editor of choice and change the `replicaCount` parameter to 10.

````bicep
using './app.bicep'

param replicaCount = 10
````

2. Commit and push the change to the parameters git repository.

2. Flux/ArgoCD detects the change and triggers Radius to redeploy the `demo` container resources with the new replica count. Radius is able to read the `app.bicepparams` file and apply the new replica count to the `demo` container resources, despite the `app.bicep` file being unmodified and also stored in a separate git repository. Note that these parameters can apply to fields within the [`PodSpec` configurations](https://docs.radapp.io/guides/author-apps/kubernetes/patch-podspec/) that Radius currently supports.

> As an SRE, I'm very happy with this experience because I did not have to do many steps to apply my parameter patch and while I had to learn some Bicep knowledge and Radius knowledge, I was able to ultimately leverage Flux or ArgoCD mechanisms which I'm familiar with to apply my patches to the Kubernetes cluster.

> As an operator or developer I'm happy to keep the configurable parameters separate from the main resource definition files so that SREs can only be granted permissions to modify the parameters and not the main resource definition files.

> As an application engineering team, we are happy with this pattern as we are already following something similar in our Terraform modules today.

_Requirements resulting from this scenario:_
- Flux and ArgoCD must be able to detect the file change described above and must be able to read the `app.bicep` and `app.bicepparams` files as required to trigger Radius to deploy resources correctly.

## Key investments
<!-- List the features required to enable this scenario. -->

### Feature 1: GitOps tooling can detect and understand Radius definition and parameter files
<!-- One or two sentence summary -->
GitOps tools (beginning with Flux and ArgoCD) must be able to read and detect the `*.bicep` and `*.bicepparams` file changes in the git repos described above as required to trigger Radius to deploy resources correctly.

### Feature 2: GitOps tooling can trigger Radius commands
<!-- One or two sentence summary -->
Once Radius definition or parameter file changes are detected and understood, GitOps tools (beginning with Flux and ArgoCD) must be able to trigger Radius to execute deployments for the Radius resources defined in the git repo.

### Feature 3: Radius can deploy resources with new parameters
<!-- One or two sentence summary -->
Radius must be able to read the `*.bicepparams` file and apply the new parameters to the `*.bicep` file resources as required to deploy the resources correctly, even if the `*.bicep` file is stored in a separate git repository. Radius must be able to orchestrate the redeployment of the resources with the new parameters even though the main `*.bicep` resource definition file was not updated.

## Key dependencies and risks
<!-- What dependencies must we take in order to enable this scenario? -->
<!-- What other risks are you aware of that need to be mitigated. If you have a mitigation in mind, summarize here. -->
<!-- Dependency Name – summary of dependency.  Issues/concerns/risks with this dependency -->
<!-- Risk Name – summary of risk.  Mitigation plan if known. If it is not yet known, no problem. -->

**Dependency: Flux, ArgoCD, etc.** - These GitOps tools/platforms must be able to detect the Radius definition files specified in the git repo and update the relevant Kubernetes clusters accordingly to deploy and manage Radius resources.

**Risk: ability for GitOps tools to deploy and manage Radius resources** - The primary risk is whether Flux, ArgoCD, etc. can trigger Radius to deploy and manage resources as specified in the git repo. We will need to test this to ensure that it works as expected.

## Key assumptions to test and questions to answer
<!-- If you are making assumptions that, if incorrect, would cause us to significantly alter our approach to this scenario, make them explicit here.  Also call out how / when you plan to validate key assumptions. -->
<!-- What big questions must we answer in order to clarify our plan for this scenario.  When and how do you plan to answer those questions (prototype feature x, user research, competitive research, etc) -->

**Assumption:** It is acceptable for SREs to have to learn some Bicep knowledge and Radius knowledge to apply patches to the Kubernetes cluster. We have validated this with potential users.

**Assumption:** It is acceptable (or maybe even desirable) for developers or operators to predefine the parameters that SREs can modify in the application configuration. This means that SREs will not be able to modify all parameters in the application configuration, but only the ones that have been parameterized into a Bicep Parameters file. We have validated this with potential users.

**Assumption:** The main resource definition files (e.g. `app.bicep`) and the parameter files (e.g. `app.bicepparams`) are stored in different git repositories (as is the case validated with users). SREs will only have access to the parameter files and not the main resource definition files, which means that if Flux/ArgoCD detects a change in the parameter file and triggers Radius to redeploy the resources, the resources will be redeployed with the new parameters but the main resource definition file will not be changed, which is acceptable to users. We need to validate that this kind of split repo setup is feasible with Radius and GitOps tools, and that Radius is able to deploy resources with the new parameters without receiving a change notification for the main resource definition file.

>Note that each Bicep parameter file declares its association with a Bicep file via the `using` keyword (e.g. `using './app.bicep'`). This is how Radius may know which Bicep file to apply the parameters to and thus which resource to redeploy.

**Assumption:** Radius is compatible with the Bicep Parameters file format and features. This will be the case once the Radius has completed its merge with the main Bicep compiler.

## Current state
<!-- If we already have some ongoing investment in this area, summarize the current state and point to any relevant documents. -->
N/A

## Notes

### July 29, 2024
- DONE: Add assumption regarding CI responsibility for publishing and pushing Recipes to registries being outside of Radius-GitOps scope.
- DONE: Clarify that version numbers are explicitly managed by operators.
- DONE: Discuss and define the approach for handling declarative deletion in Bicep deployments where parameters are deleted.

### July 16, 2024
- DONE: address some nit comments regarding accuracy of the scenarios with regards to the role of Flux/GitOps in deployments.
- DONE: add an app deployment scenario to Scenario 2
- DONE: drill deeper into Scenario 3 to specify whether we should support "patching the inputs" or "patching the outputs"
  - We should support patching the inputs, i.e. the parameters that are used to deploy the resources. This is a pattern that users are already familiar with in their Terraform modules today.

### July 9, 2024
- DONE: More specifics on the value of GitOps as well as limitations to Kubernetes
- DONE: Clarify that can use GitOps to manage other Environment objects outside of Recipes
- DONE: Move dashboard features to out of scope
- DONE: Add resource deletion feature to Scenario 2
- DONE Clarify the scope of gitops: it doesn't manage the CI (e.g. image build and app containerization) but it does manage the CD (e.g. deployment of the app to the cluster)
- DONE: Track disaster recovery scenarios as a separate feature request: DONE - https://github.com/radius-project/radius/issues/7732