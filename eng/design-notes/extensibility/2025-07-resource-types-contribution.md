# Contributing Radius Resource Types 

* **Author**: [Reshma Abdul Rahim](https://github.com/reshrahim)

## Topic Summary

This feature specification defines the experience for community members to contribute new resource types to the Radius ecosystem. Building on the foundation of Radius resource types, this feature specification provides detailed guidance for contributing to all categories of Radius resource types including core types, database resources, cache resources, messaging resources, Dapr resources, etc. The specification establishes clear pathways for community members to contribute resource types and Recipes, enabling a vibrant ecosystem of shared resources that accelerates Radius adoption.

### Top level goals

- **Democratize resource type and Recipe contribution**: Enable community members to easily contribute new resource types and associated Recipes without requiring deep knowledge of Radius internals or Go programming
- **Establish clear contribution progression**: Define explicit criteria and processes for resource types to advance through maturity levels (Alpha, Beta, Stable) to ensure quality and reliability
- **Enable ecosystem growth**: Foster a vibrant community marketplace of resource types that accelerates Radius adoption

### Non-goals (out of scope)

- Versioning and deprecation policies for community-contributed resource types.
- Migration or deprecation of existing core or portable resource types in Radius.
- Radius marketplace user interface or discovery features.
- Automated testing framework is a dependency for this feature but not part of the scope of this feature specification covered in the [Test framework for `resource-types-contrib`](https://github.com/radius-project/design-notes/pull/103) design.

## User profile and challenges

### User persona(s)

**Platform Engineers** from enterprises using Radius who want to contribute back to the community. These are individuals who have experience with infrastructure as code (Bicep/Terraform) and cloud platforms and want to share their organization's Radius resource types with the broader community. They would also like to use other community-contributed resource types in their Radius applications.

**Open Source Contributors** Developers and engineers who want to contribute to Radius as an open source project. May be new to Radius but have experience with Kubernetes, cloud platforms, or IaC. They would like to extend Radius capabilities for specific use cases or technologies.

**Vendor/ISV Contributors** Independent software vendors or cloud service providers who want to provide native Radius integration for their products. They define the best practices needed to create resource types that integrate with their services, such as databases, messaging systems, or AI/ML services.

### Challenge(s) faced by the user

- **Limited documentation and unclear pathways**: Insufficient guidance on how to contribute different types of resource types and Recipes
- **Lack of testing and validation guidance**: No clear process for testing and validating resource type contributions before submission
- **Lack of discoverability**: No clear path for sharing and discovering community-contributed resource types and Recipes

### Positive user outcomes

- Clear, step-by-step guidance for contributing resource types to Radius
- Testing and validation guidance that build confidence in contributions
- Provides a clear pathway for contributions to progress through maturity levels (Alpha, Beta, Stable)
- Recognition and visibility for contributions through community showcases
- Access to a rich ecosystem of Radius resource types and Recipes that cover diverse use cases and technologies
- Clear discoverability and documentation for all available resource types and Recipes

## Key dependencies and risks

**Dependencies**

- **Testing and Validation** A robust testing framework for validating resource type contributions.
- **Documentation** Comprehensive documentation or examples for contributing resource types and Recipes.

**Risks**
- **Quality/Security Control** Risk of low-quality or insecure resource type contributions without proper review processes
- **Maintenance Burden** Risk of abandoned or unmaintained resource types creating technical debt

## Key assumptions to test and questions to answer

**Assumptions**
- Community members are motivated to contribute resource types and will invest time in following contribution guidelines
- The Radius Resource types repository provides sufficient flexibility for most contribution scenarios

**Questions to Answer**
- How can we ensure quality of the community contributions?

  Resource types and Recipes are critical components of Radius applications, and their quality directly impacts the user experience. To ensure quality, we need to establish clear contribution guidelines, have a progression/ maturity model for the contribution and a review process that ensures contributions meet the bar. We follow a model that creates a low barrier for contributions while maintaining high standards for quality.

  >[!IMPORTANT]
  > The Radius maintainer team applies this maturity model to all core resource types (containers, gateways, secrets) developed now as part of compute extensibility feature. This ensures the Radius team validates the contribution process through direct experience, identifies gaps early, and maintains consistent quality standards across both community and maintainers. No resource type should bypass these quality gates, regardless of its origin.

    _Stage 1 : Experimental(Alpha)_

      Purpose: Enable community members to contribute resource types and Recipes with minimal barriers
      Audience: Developers/Contributors exploring new technologies or learning Radius
      Requirements:
       - Resource type schema Validation: YAML schema passes validation
       - Single Recipe: At least one working recipe for any cloud provider or platform
       - Basic Documentation: README with usage examples
       - Manual Testing: Evidence of local testing by contributor

    _Stage 2 : Verified(Beta)_

      Purpose: Ensure contributions meet production-ready standards with comprehensive testing and documentation
      Audience: Contributors seeking to have their resource types included in official Radius releases
      Requirements:
       - Multi-Platform Support: Recipes for all three platforms ( AWS, Azure, Kubernetes)
       - IaC Support: Recipes for both Bicep and Terraform
       - Automated Testing: Functional tests that validate resource type and Recipes
       - Documentation: Detailed README with Recipe coverage, troubleshooting guides, and best practices
       - Ownership: Designated owner for the resource type and Recipe
       - Maintainer Review: Formal review and approval by Radius maintainers

    _Stage 3 : Production Ready(Stable)_

      Purpose: Establish Resource types and Recipes as officially supported and maintained by the Radius project
      Audience: Enterprise users doing production deployments and seeking stable, well-tested Resource types and Recipes
      Requirements:
       - Functional tests have 100% coverage and results for Resource type schema and Recipe
       - Integration Testing: Full integration with Radius CI/CD pipeline and release process
       - Documentation: Complete user guides, troubleshooting, and best practices
       - SLA Commitment: Defined support level and response time commitments
       - Maintainer Review: Formal review and approval by Radius maintainers
    
    Contributor has to follow the contribution guidelines and the checklist for each stage to ensure that their contribution meets the requirements. Radius maintainers review the contribution meet the bar of quality for each of these stages before merging the contribution into the `resource-types-contrib` repository.

- Should we ship all resource types in this repository as part of Radius ?

  A Resource type cannot be useful without a Recipe. Every time a new resource type is added to the `resource-types-contrib` repository, it must be accompanied by at least one default Recipe that works with the resource type. Shipping just the resource type will impact the developer experience as it will cause deployment errors when there is no default Recipe available for the resource type. Based on the stages defined, we ship only resource types that have reached the `stable` stage as part of Radius. Community is encouraged to use the alpha and beta resource types and Recipes from the `resource-types-contrib` repository.

- How can we ensure maintenance of community-contributed resource types?

  - Radius maintainers triage bugs and issues reported in the `resource-types-contrib` repository. Based on the stages defined above, the Radius maintainers will prioritize bugs and issues reported for resource types and Recipes
     - For bugs on existing resource types and Recipes, we follow the below process:
        - If the resource type is in stable stage, the Radius maintainers will prioritize fixing these bugs and ensuring the resource types remain functional and up-to-date.
        - If the resource type is in beta/ alpha stage, the contributor will be responsible for fixing the bug and ensuring the resource type remains functional and up-to-date. 
    - For proposals on new resource types, we triage the proposal and assign it to the contributor who proposed the resource type if interested to contribute or leave it open for other contributors to pick up.

- What level of technical support should be provided to contributors?
   -  Detailed guidelines and templates for contributing resource types and Recipes
   -  Clear documentation on testing and validation procedures
   -  Radius Discord server contribution channel for community support and discussion

- How can we incentivize contributions?
  - Recognition through contributor showcases on the Radius website or documentation
  - Swags for contributors who submit meaningful contributions
  - Opportunities for contributors to become maintainers of the `resource-types-contrib` repository

## Current state

Currently, community contributions are limited to Recipes for portable Resource types, restricting ecosystem growth. While the Radius Resource types feature enables any service or technology to be modeled as a resource type, the contribution process lacks clear definition and community-contributed resource types have no established discovery mechanism.

## Details of user problem

As a platform engineer who has developed custom resource types for my organization's use of Radius, I want to contribute these resource types back to the community so that the community can benefit from my work.

When I try to contribute a new resource type, I struggle to understand the different pathways available. Should I create a Radius resource type, contribute a Recipe, or modify the core Radius codebase?

I lack clear guidance on how to structure my contribution. There's no template or example to follow, and I'm unsure about required documentation, testing procedures, or quality standards.

I don't know how to test my resource type before submitting a PR.

Also, how do I discover all the contributed resource types and Recipes?

## Desired user experience outcome

After the contribution experience is implemented, I can easily contribute resource types to the Radius community through a clear, well-documented process. I can choose from multiple contribution pathways based on my needs and technical expertise, with each pathway providing comprehensive guidance.

I can validate my resource type and its associated Recipes before submitting it to the community.

Once I contribute a resource type, it becomes discoverable in the repository, where other users can find, evaluate, and use it. 

### Detailed user experience

#### User story 1: As a platform engineer or an open-source contributor, I want to contribute a Radius Resource type so that I can share my work with the community and help others benefit from it.

1. The contributor understands the contribution guidelines at the `resource-types-contrib` repository and familiarizes themselves with the [contribution process](https://github.com/reshrahim/resource-types-contrib/blob/main/CONTRIBUTING.MD)

1. They will pick the resource type that they want to contribute either from the list of open issues in the `resource-types-contrib` repository, or they can contribute their own resource type if it does not already exist.

    > [!NOTE]
    > We create a list of open issues in the `resource-types-contrib` repository that needs contribution starting with the portable Resource types and then move open requests for new resource types from `radius` repository to the `resource-types-contrib` repository.
    >
    >    | Namespace | Resource Type | Description |
    >    |---|---|---|
    >    | Radius.Data | sqlDatabases | Relational database support |
    >    | Radius.Data | redisCaches | In-memory data store and cache |
    >    | Radius.Data | mongoDatabases | NoSQL document database |
    >    | Radius.Data | rabbitMQQueues | Message queue system for asynchronous communication |
    >    | Radius.Dapr | stateStores | Distributed state management via Dapr |
    >    | Radius.Dapr | pubSubBrokers | Message publishing and subscription via Dapr |
    >    | Radius.Dapr | secrets | Secret management via Dapr components |
    >    | Radius.Dapr | configurationStores | Configuration management via Dapr |

1. Create a new directory for your resource type under the `resource-types-contrib` repository, following the established directory structure.

    For e.g. If you are contributing to a `redis` resource type, the directory structure would look like this:
    
        resource-types-contrib/
        ├── README.md
        ├── datastores
        │   ├── redis/  
        │   │   ├── README.md  # Documentation for the redis resource type
        │   │   ├── redis.yaml/  # Schema definition for the redis resource type
        │   │   ├── recipes/     # Recipes for the redis resource type
        │   │   │       ├── aws-memorydb/
        │   │   │       │       ├── bicep/
        │   │   │       │       │       ├── aws-memorydb.bicep
        │   │   │       │       │       └── aws-memorydb.bicepparams
        │   │   │       │       └── terraform/
        │   │   │       │               ├── main.tf
        │   │   │       │               └── var.tf
        │   │   │       ├── azure-rediscache/
        │   │   │       │       ├── bicep/
        │   │   │       │       │       ├── azure-redis.bicep
        │   │   │       │       │       └── azure-redis.bicepparams
        │   │   │       │       └── terraform/
        │   │   │       │               ├── main.tf
        │   │   │       │               └── var.tf
        │   │   │       └── kubernetes-rediscache/

1. Define the `redis.yaml` schema file for your resource type. Follow the contribution guidelines for the resource type schema.

    ```yaml
    name: Radius.Datastores
    types:
      redisCaches:
        apiVersions:
          '2025-07-20-preview':
            schema: 
              type: object
              properties:
                environment:
                  type: string
                application:
                  type: string
                capacity:
                  type: string
                  description: The capacity of the Redis Cache instance.
                host:
                  type: string
                  description: The Redis host name.
                  readOnly: true
                port:
                  type: string
                  description: The port number of the Redis instance.
                  readOnly: true
                username:
                  type: string
                  description: The username for the Redis instance.
                  readOnly: true
                password:
                  type: string
                  description: The password for the Redis instance.
                  readOnly: true
            required:
                - environment
    ```

    **Resource Type Schema Enforcement**

    Radius enforces ARM naming conventions for resource type schemas. The following guidelines need to be followed:

    - The `name` field follows the format `Radius.<Category>`, where `<Category>` is a high-level grouping (e.g., Datastores, Messaging, Dapr) For e.g. Radius.Datastores. This is a change from the previous format of `Applications.Datastores` to help users distinguish their own resource types from the Radius Resource types. The core resource type will also need to follow the `Radius.Core` format so that it is consistent across the resource types.

    - The resource type name follows the camel Case convention and is in plural form, such as `redisCaches`, `sqlDatabases`, or `rabbitMQQueues`.

    - Version should be the latest date and follow the format `YYYY-MM-DD-preview`. This is the date on which the contribution is made or when the resource type is tested and validated. For e.g. `2025-07-20-preview`.

    - Properties should follow the camel Case convention and include a description for each property. 
        - `readOnly:true` set for property automatically populated by Radius Recipes.
        - `type` could be `integer`, `string` or `object`; Support for `array` and `enum` in progress
        - `required` for required properties. `environment` should always be a required property.

1. Create Recipes for your resource type in the `recipes` directory. Each Recipe should be organized by cloud provider or technology stack, such as `aws-memorydb`, `azure-redis`, or `kubernetes`. 

    - Each Recipe should include a README.md file that describes how to use the Recipe, including prerequisites, parameters required, and examples.
    - The Recipes could be a Bicep or Terraform template organized in the respective directories, such as `bicep` or `terraform`.
    - Each Recipe should handle secrets securely and can use the core Resource types like containers, gateways, and secrets when possible
    - Contributors are required to add at least one Recipe that works with the resource-type but also encouraged to provide multiple Recipes for different cloud providers or technologies, such as AWS MemoryDB, Azure Redis Cache, or Kubernetes Redis Cache.
    - Guidelines documentation for Recipes is available [here](https://github.com/Reshrahim/resource-types-contrib/blob/main/contributing-docs/contributing-resource-types-recipes.md#recipes-for-the-resource-type)

1. Manually test the resource type and Recipe locally. Detailed instructions for testing the resource type and Recipe are written [here](https://github.com/Reshrahim/resource-types-contrib/blob/main/contributing-docs/testing-resource-types-recipes.md)

1. For an alpha stage contribution, the contributor provides evidence of local testing but for other stages, the contributor must add a functional test for the resource type and Recipe in the `radius` repository to validate the resource type schema and the Recipes provided. The tests will ensure that the resource type can be created, updated, and deleted successfully, and that the Recipes can be deployed without errors.

1. Document the resource type and Recipe in the `README.md` file of the resource type directory. The documentation should include:

    - Overview of the resource type and its purpose
    - Schema definition and properties
    - Recipes available for the resource type
    - How to use the resource type and Recipes
    - Examples of usage

1. Submit a pull request (PR) to the `resource-types-contrib` repository with the changes and the documentation. Make sure to follow the detailed checklist

    Before submitting your contribution. Make sure to check the following:

    - Resource type schema follows naming conventions
    - All properties have clear descriptions
    - Required properties are properly marked
    - Read-only properties are marked as `readOnly: true`
    - Recipes are provided for at least one platform
    - Recipes handle secrets securely
    - Recipes include necessary parameters and outputs
    - Recipes are idempotent and can be run multiple times without issues
    - Recipes output necessary connection information
    - Test your resource type and recipes locally
    - Documentation is complete and clear

1. The PR will be reviewed by the Radius maintainers, who will provide feedback and request changes if necessary. 

    - The Radius Maintainer/ Approver will do a quick check on the authenticity of PR and the contributor and kick off the functional tests for the resource type and Recipe.
        - The functional tests will validate the resource type schema and the Recipes provided. The tests will ensure that the resource type can be created, updated, and deleted successfully, and that the Recipes can be deployed without errors.
    - The contributor should address the feedback and make the necessary changes to the PR.
    - Once the PR is approved, it will be merged into the `main` branch of the `resource-types-contrib` repository.

The end-end contribution guidelines are documented in the [here](https://github.com/Reshrahim/resource-types-contrib/)

>[!NOTE]
> The user story above covers the contribution of a new resource type with a Recipe to the `resource-types-contrib` repository. Users can also edit existing resource types and Recipes by following the same contribution process. The contributor should follow the same steps as above, but instead of creating a new directory, they should edit the existing resource type directory and update the schema, Recipes, and documentation as needed.

## Key Investments

## Contribution guidelines

- Clear guidelines for contributing resource types and Recipes
- Support for multiple contribution pathways (e.g., resource types, Recipes, etc.)
- Templates and examples for resource type schemas and Recipes
- Detailed documentation on testing and validation procedures
- Contributor checklist for each stage of contribution (Alpha, Beta, Stable)

## Contributor Recognition

- Swags for contributors who submit meaningful contributions
- Contributor showcase on the Radius website or documentation
- Opportunities for contributors to become maintainers of the `resource-types-contrib` repository

## Migration from Current State

  ### Phase 1: Parallel Operation
  - Keep existing `recipes` repository active
  - Start populating `resource-types-contrib` repository with new resource types and Recipes
  - Redirect contributions to new `resource-types-contrib` repository

  ### Phase 2: Migration
  - Migrate existing Recipes from the `recipes` repository to the `resource-types-contrib` repository
  - Update Radius codebase to use `resource-types-contrib` repository for resource types and Recipes
     - Changes to `local-dev` Recipes to be registered from the `resource-types-contrib` Recipe path
     - Remove `portable` resource types implementation from Radius repository and update to use `resource-types-contrib` repository
  - Update all references in Radius documentation to point to the new `resource-types-contrib` repository

  ### Phase 3: Sunset
  - Archive existing `recipes` repository
  - Notify community about the migration and deprecation of old Recipes
  - Remove Recipes from Radius repository

## Future Considerations

- **Scaffolding Tooling**: Building a CLI tool or scaffolding generator to help contributors quickly set up the directory structure, schema files, and Recipes for new resource types
- **Recipe generation with AI**: Using AI to convert one Recipe format to another (e.g., Bicep to Terraform) or to convert one platform Recipe to another (e.g., AWS MemoryDB to Azure Redis Cache)
- **Marketplace Integration**: Integration with Artifact Hub or other marketplace solutions to enable easy discovery and installation of Radius Resource types and Recipes

