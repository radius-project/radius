# Functional Test Guidelines

This document outlines guidelines for when to add functional tests, what features should be tested, and what user scenarios each test should cover. For the purposes of this document, we’re going to draw a distinction between two categories of functional tests: E2E tests and feature-level tests.  

E2E tests cover a complete user scenario that utilizes an application. An example would be deploying a MongoDB resource. When doing so, a user has to set up an application, environment, deploy the app, ensure the connection succeeds, and eventually delete the app. This would be considered an E2E scenario since we are testing the entire life cycle of a resource/feature. Most resource-driven tests (i.e. Redis, Mongo, Containers, etc) will be E2E tests.  

Feature-level tests cover targeted components of the code. An example would be many of the CLI tests. The purpose of these tests is to check very specific scenarios (i.e. validating application deletion, testing Recipe commands, etc).  

## Guidelines

Generally, anything related to the outcomes a user might care about should be tested. Examples of this include deploying an application, using a Recipe, deploying a resource, etc. Outcomes that a user would not necessarily care about could be something like verifying that the Redis processor is working; this is a backend process that most users would not need to understand, unless directly contributing to the project. Anything in bold in this section is a guiding principle.  

### Deciding when to use functional tests 

**Any major behavior of Radius should be tested.**  

We want to cover most testing through unit tests. However, there are features that crosscut multiple resources and components (i.e. Recipes is a feature that extends to multiple resources) that are hard to cover with unit tests. These types of behaviors should be covered in functional tests.  

It’s important to analyze the nature of the work being done to determine if a functional test is needed. This is largely up to the discretion of the developers to decide, but if any of these apply to the work that is being done, a functional test may be needed: 

1. The feature has a high complexity (has many dependencies, tight coupling to internal or external dependencies, uses multiple services, affects many resources) 
Examples: Terraform, Recipes, IAM roles, Kubernetes metadata 
2. Any feature that makes use of async jobs
Examples: Resource deployment (especially related to portable resources) 
3. Any feature that has a high customer impact
Examples: Connecting to a customer’s MongoDB, deleting a portable resource that connects to a customer’s existing data store  
4. Any feature that can be applied to multiple resources. This includes features that make use of shared code or introduce shared code**  
Examples: IAM roles, Recipes, deleting resources, deploying resources  
5. Any feature that requires heavy user interaction 
Examples: CLI testing 

**If a component or feature is found to work in ways not intended, we need to add additional tests or update existing ones.**  

Unknowns are always going to come up when developing or updating complex features. If we start to find that these features are working unexpectedly, it’s best that we add functional tests to verify and monitor the intended behavior. An example of this can be seen with the default Recipe experience and Portable Resource refactoring. The delete functionality was refactored heavily and functional tests were written that only covered Kubernetes deployments and deletions. When tested with Azure deployments, however, it was found that Azure resource deletion was failing. In this case, we have two options: add a feature-level test covering resource deletion or update a portable resource E2E test to use a non-Kubernetes deployment. Either way, we’re removing the gap that was found with the feature.  

### Deciding what scenarios to cover with functional tests 

**Functional tests must cover distinct user scenarios and outcomes. The specific scenarios will be determined based on the feature.**  

Functional tests are used to test user outcomes. We need to approach defining scenarios from a user’s perspective. How might a user use Radius in their day-to-day development? An example of a user scenario is deploying an application to Radius. Within this scenario, a user must install and set up Radius, define a bicep template with the application information, deploy the template, and verify the application gets created. Eventually, they’ll want to delete the application too. All these steps encompass one scenario. Anything that lies outside of what a user might care about should be covered by unit tests, not functional tests.  

This largely applies to E2E tests; feature-level tests may not require all these steps, but they should still cover a scenario. For example, in the CLI functional tests we verify Recipe CLI commands (register, list, show, etc), but don’t deploy the Recipe or use it with a portable resource. The scenario here is that we’re verifying Recipe CLI commands work, but we’re not necessarily testing the lifecycle of a Recipe through deployment, validation, and deletion.   

Each of the chosen scenarios for any functional test must be distinct. Let’s use a Redis cache as an example. This can be deployed by specifying properties manually or by using a Recipe. These are the two distinct user scenarios. Manual provisioning requires the user to provide all connection values to the cache within their bicep template for deployment to succeed. No additional work is needed outside of defining the Redis resource. Using a Recipe requires a different setup. The user must define a Recipe, register the Recipe to the environment, and then specify that the portable resource should use the Recipe. From a user scenario perspective, these scenarios cannot be treated the same and would both need to be tested.  

**When in doubt of how to determine the boundaries of what scenarios are considered unique, we will make use of equivalency cases.**  

This guideline is best explained with an example. Sometimes, it’s hard to determine what scenarios would be considered distinct. Let’s go back to the Redis cache example. Redis caches can be deployed using multiple providers. Radius currently supports self-hosted, Azure, and AWS providers. We could, in theory, test Redis with each provider. But how do we determine that each of these is a unique scenario that requires its own test? If we were to use Azure as the provider, we register Azure credentials, define a Recipe with an Azure cache and add a connection to that cache in our bicep template. If we instead choose to use AWS as the provider, we still register AWS credentials, define a Recipe with an AWS cache and add a connection to that cache in our bicep template. The details in the templates and the way we set up our cluster/environment/etc may slightly differ because of using a different provider, but the steps in each of these scenarios are mostly equivalent. Based on this, provider-specific tests wouldn’t be considered unique scenarios. Instead, we should pick one candidate out of the providers and test that.  

Negative testing: We normally don’t have negative cases for functional tests, except in select scenarios. Most negative cases are covered by unit tests. However, if a negative testing is needed, we can use equivalency casing to determine which scenarios to cover. For example, defining a string for a Redis port in a bicep template would return an error. Inputting a string for a TLS boolean input in the template would return a very similar error. However, these reduce to an incorrect input causing an error in deployment, so they don’t need to be tested separately. It’s enough to test one of these cases.  

### How to write functional tests 

**When writing functional tests, the setup of Radius and other platforms used (i.e. Kubernetes, Azure, AWS, etc) should be identical to what a user would need when running Radius locally.**   

The purpose of functional tests is to mimic what a user may do when using Radius. It is important that we follow a setup that is as identical as possible to what a user may do. This is also why the functional tests make use of the CLI instead of directly calling APIs for deployment, deletion, etc.  

**When testing E2E scenarios, we need to verify that key steps are working correctly.**  

This goes back to how we define user scenarios. Key steps in an E2E scenario may include successful deployment of all resources, connection to resources, and deletion of the application, environment, or resource. These should all be validated when running functional tests.  

### E2E vs. Feature-Level Tests 

Deciding between the two categories of functional tests might not always be intuitive. However, there are some questions we can ask that will help determine which type of functional test to use. 

1. Are there sub-components or areas that a unit test cannot cover for a feature? 
If there are any gaps in unit testing, we should add a feature-level test. An example of this would be a lot of CLI features or anything that makes use of async calls. We can mock user calls and interactions but it’s hard to cover scenarios where the user is interacting with the CLI without the use of functional tests.  
2. Is this feature or addition something that a user would care about or need to know how it works? 
If yes, an E2E test is needed. If no, we need to analyze if a feature-level test is needed. If we find that a feature isn’t working as expected or unit tests aren’t enough, that means that our test coverage/scenarios are lacking, and we should add a feature-level test (i.e. the Azure deletion bug is an example of when we may want to add another feature-level test for delete) 
3. Are there additional ways that a feature may work that would lie outside of the scope of a distinct user scenario?  
Let’s look at the delete resource scenario again. Deleting a resource is a distinct user scenario. However, cloud deletion may be handled differently than a local resource deletion. In this case, we need to look into adding a feature-level test to cover cloud scenarios. 

### Examples of applying guidelines  

Any Radius resource must have functional tests. Resources are the easiest example of when to use E2E functional tests. These are a major way that users will interact with Radius so we need to test their lifecycles from deployment to deletion. It is not enough to just test specific components for these resources. The number of functional tests needed per resource is determined based on distinct user scenarios for using each resource.  

Any resource with Recipe support (i.e. portable resources) must test for both manual and Recipe resource provisioning cases. This example was discussed earlier. Other resources may need further analysis, but we have defined unique scenarios for portable resources.  