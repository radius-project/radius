# GitHub Radius Feature Specification

GitHub Radius is a prototype version of Radius directly integrated into GitHub repositories and GitHub Copilot. It is built using:

- Open-source Radius with optimizations for running in GitHub Actions

- Agents, skills, and/or MCP servers for integration with GitHub Copilot

- Mock-up integration with the GitHub GUI

GitHub Radius enables GitHub users to define applications and environments within their GitHub repository, and to deploy applications directly from the GitHub GUI to AWS and Azure.

The GUI is built using a mock-up of the GitHub GUI. In the fullness of time, the GUI UX will also be implemented in the Radius Dashboard and Headlamp.

## User Journeys

There are several top-level user journeys in scope for this prototype:

1. As an open-source developer, I want to deploy an application stored in a public GitHub repository to my cloud environment. I expect GitHub Radius to build and deploy the application:
    1. Using AWS
    1. Using Azure

1. As a developer, I want to define multiple environments in my GitHub repository and deploy to each environment.

1. As a developer, I expect GitHub Radius to visually highlight where my code changes were made and the impact of the change in pull request and diffs.

The following user journeys are out of scope for the initial prototype, but will be in scope in the future:

- As an open-source developer, I want to deploy an application stored in a public GitHub repository to my cloud environment. I expect GitHub Radius to build and deploy the application using <u>Google Cloud</u>.

- As a developer, I want to use GitHub Copilot to build a new application from scratch. I expect Copilot to create a GitHub
    repository then deploy and test the application in my cloud environment.

- As a developer, I want to transition from using GitHub Radius, to a self-hosted, Kubernetes version of Radius.

## User Journey 1.1: Deploy an open-source application to AWS

### Step 1: Discovery

1. The user visits a public repository on GitHub and decides to deploy it to their AWS account. They see an inactive Deploy button.

    ![image1](2026-03-github-radius-feature-spec/image1.png)

1. The user clicks on the **Deploy** button to discover what this Deploy feature could be.

    ![image2](2026-03-github-radius-feature-spec/image2.png)

    The user does not have Admin permission to the repository, so they are instructed to create a fork. The user clicks **Create a fork**.

1. A fork of the repository is created in their GitHub account.

###  Step 2: Defining an application

1. The user clicks the **Deploy** button again. The user sees that they can now define an application.

    ![image3](2026-03-github-radius-feature-spec/image3.png)

    The user clicks **Define an Application.**

1. The user is taken to the Copilot web interface with a prompt already given to create an application definition.

    ![image4](2026-03-github-radius-feature-spec/image4.png)

    In the background, Copilot is first reading the Radius platform constitution. This is a markdown file which instructs Copilot on how to model cloud-native applications. The Radius platform constitution is maintained by the Radius team and cannot be customized for now. The constitution has the following sections:

    - **Application architecture patterns**. A set of pre-defined architectural patterns. The purpose of these patterns is to prevent arbitrary architectures and ensure applications are composable using existing resource types. This will likely include:
      - Stateless web/API service
      - Stateful/database-backed application
      - Event-driven application
      - Batch job
      - Streaming/real-time processing application
    - **Resource types**. The set of resource types to use when constructing an application. The purpose is to standardize resource abstractions across cloud platforms. In the Radius constitution, this will likely be a link to a manifest of pre-defined Radius Resource Types.
    - **Resource composition rules**. For each Resource Type, a set of rules that define how resources can be combined. For example, a web service can connect to a database, but not the reverse.
    - **Resource dependencies**. For each Resource Type, a set of rules that define required dependencies. For example, a Container requires there to be a container image, an OCI registry, and a Kubernetes cluster. A MySQL database requires there to be a virtual network.
    - **Naming conventions**. Guidance for naming the Bicep symbolic name and the actual resource name.
    - **Secrets**. Defines how to properly handle secrets and how to generate secret values.

    Radius then identifies the resources required by this application and outputs an `app.bicep` file in the  `.radius` directory.

    The user goes back to the repository page and sees that an application has been defined.

    ![image5](2026-03-github-radius-feature-spec/image5.png)

1. The user clicks on **todo-list-app** and is taken to a visualization of the application definition.

    ![image6](2026-03-github-radius-feature-spec/image6.png)

    The user clicks back to the main repository page.

###  Step 3: Creating an AWS environment

1. The user clicks Deploy and sees options for creating AWS, Azure, or Google Cloud environments. The user clicks **Create AWS environment**.

    ![image7](2026-03-github-radius-feature-spec/image7.png)

1. A new window opens for creating an AWS environment.

    ![image8](2026-03-github-radius-feature-spec/image8.png)

1. The user clicks **Create trusted OIDC identity provider and IAM  role**. The AWS console opens in a new window. The user is  prompted to login to their account.

    ![image9](2026-03-github-radius-feature-spec/image9.png)

1. A CloudFormation stack is opened. The user reviews the stack then  clicks Create stack.

    ![image10](2026-03-github-radius-feature-spec/image10.png)

    This CloudFormation stack is stored in a Radius-maintained S3 bucket. It creates an IAM OIDC Identity Provider, similar to running this command:

    ```bash
    aws iam create-open-id-connect-provider \
      --url https://token.actions.githubusercontent.com \
      --client-id-list sts.amazonaws.com \
      --thumbprint-list 6938fd4d98bab03faadb97b34396831e3780aea1
    ```

    This registers GitHub Actions as a trusted identity provider. The CloudFormation stack also creates an IAM role similar to this
    command:

    ```bash
    aws iam create-role --role-name radius-{owner}-{repo} \
      --assume-role-policy-document <trust-policy>
    ```

    The trust policy is similar to:

    ```json
    {
      "Version": "2012-10-17",
      "Statement": [
        {
          "Effect": "Allow",
          "Principal": {
            "Federated": "arn:aws:iam::{account}:oidc-provider/token.actions.githubusercontent.com"
          },
          "Action": "sts:AssumeRoleWithWebIdentity",
          "Condition": {
            "StringEquals": {
              "token.actions.githubusercontent.com:aud": "sts.amazonaws.com"
            },
            "StringLike": {
              "token.actions.githubusercontent.com:sub": "repo:{owner}/{repo}:*"
            }
          }
        }
      ]
    }
    ```

    It then attaches a customer-managed IAM policy to the new role similar to:

    

1. The user returns to the *Create an AWS environment* page. They enter the environment name, IAM role ARN, select the region, then click **Confirm authentication**.

    ![image11](2026-03-github-radius-feature-spec/image11.png)

    When the user clicks Confirm authentication:

    - A GitHub environment is created in the repository
    - Metadata is added to the environment, possibly as an environment-level variable, including:
      - AWS Account ID
      - AWS Region
      - AWS IAM Role ARN
    - A workflow is dispatched which performs an AWS login test and confirms the adequate IAM permissions are available

1. While the workflow is running, there is a visual indication that it is running in the background. Once complete the *Check environment dependencies >* button is enabled. The User clicks **Check environment dependencies >** button.

###  Step 4: Defining recipe parameters

1. Radius examines the resources in app.bicep and the default recipes for each resource type. The user is presented with each required recipe parameter to set on the environment. In the case of todo-list-app:

    - **Radius.Compute/containers** has a required parameter for the Kubernetes cluster and namespace
    - **Radius.Compute/containerImages** has a required parameter for OCI registry URL
    - **Radius.Data/mySqlDatabases** has a required parameter for the VPC and list of subnets

    ![image12](2026-03-github-radius-feature-spec/image12.png)

    Radius prepopulates the drop-down boxes with valid values by making API calls to list relevant resources from the user's AWS account. For example, the list of EKS clusters is prepopulated for the user to select from (however namespace is left blank since there is no AWS API call to list Kubernetes namespaces).

1. The user returns to the GitHub repository.

###  Step 5: Deployment

1. The user clicks **Deploy** again. The user sees that there is now a `dev` environment setup with their AWS account.

    ![image13](2026-03-github-radius-feature-spec/image13.png)

1. The user clicks the `dev` environment.

    ![image14](2026-03-github-radius-feature-spec/image14.png)

    The user is redirected to the deployment dashboard and monitors the deployment. The user is prompted which application to deploy (not visualized here). Since there is only a single application in this repository, the user only has to click **Deploy** and the deployment begins.

    ![image15](2026-03-github-radius-feature-spec/image15.png)

    Resources queued for deployment are marked in gray. Resources being deployed are yellow. Resources successfully deployed are green. Resources that failed to deploy are red.

    When the user returns to the main repository page, they now see a deployment with the environment and timestamp under Deployments.

    ![image16](2026-03-github-radius-feature-spec/image16.png)
