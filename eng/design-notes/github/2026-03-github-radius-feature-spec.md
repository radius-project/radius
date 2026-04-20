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

1. The user visits a public repository on GitHub and decides to deploy it to their AWS account. They see a Deploy button.

    ![image1](2026-03-github-radius-feature-spec/image1.png)

1. The user clicks on the **Deploy** button to discover what this Deploy feature could be.

    ![image2](2026-03-github-radius-feature-spec/image2.png)

1. The user creates a fork of the repository in their GitHub account.

###  Step 2: Creating an AWS environment
1. The user clicks Deploy and sees options for creating AWS, Azure, or Google Cloud environments. The user clicks **Create AWS environment**.

    ![image3](2026-03-github-radius-feature-spec/image3.png)

1. A new window opens for creating an AWS environment.

    ![image4](2026-03-github-radius-feature-spec/image4.png)

1. The user clicks **Create trusted OIDC identity provider and IAM  role**. The AWS console opens in a new window. The user is  prompted to login to their account.

    ![image5](2026-03-github-radius-feature-spec/image5.png)

1. A CloudFormation stack is opened. The user reviews the stack then  clicks Create stack.

    ![image6](2026-03-github-radius-feature-spec/image6.png)

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

1. The user returns to the *Create an AWS environment* page. They enter the environment name, IAM role ARN, select the region, then click **Confirm authentication**.

    ![image7](2026-03-github-radius-feature-spec/image7.png)

    When the user clicks Confirm authentication:

    - A GitHub environment is created in the repository
    - Metadata is added to the environment, possibly as an environment-level variable, including:
      - AWS Account ID
      - AWS Region
      - AWS IAM Role ARN
    - A workflow is dispatched which performs an AWS login test and confirms the adequate IAM permissions are available

1. While the workflow is running, there is a visual indication that it is running in the background. Once complete the *Define environment dependencies >* button is enabled. The User clicks **Define environment dependencies >** button.

###  Step 3: Defining environment dependencies

1. Radius prompts the user for common environment dependencies. These dependencies should cover the majority of cloud-native applications. However, in the future, this will need to be made more extensible. Today, these include:

    - Container platform: provide a dropdown box of EKS and ECS clusters in the account/region and prompt for the Kubernetes namespace
    - OCI registry: provide a dropdown box with (1) this repositories GHCR, and (2) the ECR registry for that account/region
    - VPC: a dropdown box with the VPC in that account/region
    - Subnets: a dropdown box with the subnets available for the selected VPC

    ![image8](2026-03-github-radius-feature-spec/image8.png)
    
    Radius prepopulates the drop-down boxes with valid values by making API calls to list relevant resources from the user's AWS account. For example, the list of EKS clusters is prepopulated for the user to select from (however namespace is left blank since there is no AWS API call to list Kubernetes namespaces).

1. The user clicks **Create AWS Environment** then returns to the GitHub repository. In the background, a GitHub Environment is created with environment variables use to store the collected metadata.

###  Step 4: Deployment

1. The user clicks **Deploy** again. The user sees that there is now a `dev` environment setup with their AWS account. The `dev` environment is decorated with a green checkmark to indicate the authentication and authorization has been tested.

    ![image9](2026-03-github-radius-feature-spec/image9.png)

1. The user clicks the `dev` environment.

    ![image10](2026-03-github-radius-feature-spec/image10.png)

1. A Copilot agent is dispatched to define the application using the resource types built into Radius. The user is presented with the modeled application graph.

    ![image11](2026-03-github-radius-feature-spec/image11.png)

1. After the user tells Copilot yes to deploy, the user is redirected to the deployment dashboard and monitors the deployment. 

    ![image12](2026-03-github-radius-feature-spec/image12.png)

    Resources queued for deployment are marked in gray. Resources being deployed are yellow. Resources successfully deployed are green. Resources that failed to deploy are red.

    When the user returns to the main repository page, they now see a deployment with the environment and timestamp under Deployments.

    ![image13](2026-03-github-radius-feature-spec/image13.png)


###  
