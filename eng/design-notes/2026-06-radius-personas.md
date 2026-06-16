# Radius User Personas

## Overview

This document defines the two primary personas that Radius serves: Developers and Platform Engineers. These definitions follow common cloud native community usage, including the [CNCF Platforms Working Group](https://tag-app-delivery.cncf.io/whitepapers/platforms/) and the broader platform engineering movement. The goal is to establish a shared vocabulary for product, design, documentation, and outreach so that messaging and feature decisions consistently map back to who we are serving and the outcomes they care about.

Personas are an abstraction, not a job title. Real people rarely fit neatly into a single box. A Developer at a small startup may also run the cluster, and a Platform Engineer may still write application code. There is always some overlap between any two personas. The distinction below is about primary focus and the problems each persona is accountable for solving, not a rigid boundary.

## Developers

Developers build, ship, and operate the application code that delivers business value. They work across frontends, backends, APIs, and the data services those applications depend on. Their day is measured in features delivered and incidents resolved, and they want fast feedback loops that let them move from idea to running software without becoming experts in infrastructure.

In the cloud native community, this persona is often called an application developer or product engineer. They are the consumers of the internal developer platform: they want a paved path that lets them self-serve the infrastructure their application needs while staying within the guardrails their organization has defined.

### Profile

- Focused on application logic, services, and the dependencies those services consume, such as databases, caches, message queues, and API gateways.
- Comfortable with CLIs, SDKs, source control, and CI/CD, but prefer not to manage low-level cloud or Kubernetes details directly.
- Want consistent local, test, and production experiences so that "works on my machine" maps cleanly to "works in production".
- First responders to test failures and production incidents for their services.
- Adopt tools quickly when they reduce friction and have a shallow learning curve.

### Example use cases

- Define an application and its dependencies, such as a container plus a Postgres database and a Redis cache, then deploy it to a development environment with a single command.
- Self-serve infrastructure through approved Recipes without writing Terraform or Bicep or filing a ticket and waiting on another team.
- Use the application graph to understand how services and infrastructure connect and to trace the blast radius of a change.
- Promote the same application definition from a local environment to staging and production without rewriting it for each target.
- Connect a service to a dependency by name and let the platform wire up connection strings, secrets, and permissions.

## Platform Engineers

Platform Engineers design and operate the internal developer platform that Developers build on. They create the reusable templates, paved paths, and self-service infrastructure that let many Developers move quickly while staying compliant with organizational standards for security, cost, and operations. Their customers are the Developers inside their own organization.

In the cloud native community, this persona overlaps heavily with DevOps, Site Reliability Engineering, and infrastructure roles. In larger organizations these are often distinct teams; in smaller ones a single person may cover platform engineering, operations, and on-call. What unifies the persona is accountability for the platform as a product: standardization, automation, reliability, and developer enablement.

### Profile

- Responsible for environments, infrastructure provisioning, and the guardrails that keep deployments consistent and compliant.
- Fluent in infrastructure-as-code, Kubernetes, cloud providers, and CI/CD systems.
- Act as gatekeepers for new technology adoption, evaluating tools against long-term strategy, security posture, and operational cost.
- Measured by developer productivity, platform reliability, and reduced operational toil, not by application features.
- Adopt foundational tools deliberately, because the choices they make become standards across every team.

### Example use cases

- Author and register Recipes that encode best practices for security, cost, and compliance, so Developers consume infrastructure without needing to know the underlying implementation.
- Define logical environments such as development, test, and production, what specific cloud account and location the environment is mapped to, and define what Recipes are used to deploy to that environment.
- Swap the implementation behind a resource type, for example moving a database Recipe from a Kubernetes-hosted Postgres to a managed cloud database, without requiring Developers to change their application.
- Enforce organizational policy and standards centrally so that every application deployed through the platform is compliant by default.
- Integrate Radius into existing CI/CD pipelines and existing catalogs of infrastructure-as-code templates for incremental adoption.

## Other personas

The personas below are stakeholders whose needs influence Radius, but Radius is not directly designed for them today. They are served indirectly through the workflows of Developers and Platform Engineers rather than through features built specifically for them. We call them out so their interests are represented in product and outreach decisions without overstating how directly Radius targets them.

### Operators / SREs

Operators and Site Reliability Engineers keep running systems healthy. They own reliability targets, on-call rotations, observability, and incident response for applications already in production. In the cloud native community this persona is distinct from Platform Engineering: SREs operate what the platform produces rather than building the platform itself, though the two roles overlap heavily and are often combined on smaller teams.

Radius is not designed primarily as a day-two operations tool, but Operators and SREs benefit indirectly from capabilities built for the primary personas, such as the application graph for understanding dependencies and blast radius, the dashboard for visualizing deployed applications, and consistent environment definitions that reduce configuration drift. Their reliability and observability requirements are largely satisfied through the existing investments of Developers and Platform Engineers rather than through SRE-specific features.

### Security / Compliance Engineers

Security and Compliance Engineers define the policies, controls, and standards that applications and infrastructure must meet. They care about least-privilege access, secret handling, supply-chain integrity, and auditable, compliant-by-default deployments.

This persona typically shapes requirements rather than using Radius directly. Their goals are met through the guardrails Platform Engineers encode in Recipes and environments, which let an organization enforce security and compliance standards centrally so that every application deployed through the platform is compliant by default. Radius gives this persona leverage by making their policies the paved path, but it does not provide a dedicated security or compliance interface today.

## Where the personas overlap

Radius is explicitly designed around the collaboration between Developers and Platform Engineers, so overlap between these two primary personas is expected and healthy rather than a problem to eliminate. The shared application and environment model is the contract between them: Platform Engineers define what is possible and compliant, and Developers consume it to ship software. The stakeholder personas in the previous section sit at the edges of this collaboration, influencing the guardrails and consuming the results rather than driving the day-to-day workflow.

Common areas of overlap include:

- **Authoring infrastructure templates.** A senior Developer may contribute Recipes, and a Platform Engineer may write application definitions to validate the paved path they are building.
- **Small teams and startups.** A single engineer often plays both roles, building the application and operating the platform it runs on, and may also cover operations, security, and on-call.
- **Incremental adoption.** During onboarding, Developers and Platform Engineers frequently pair to integrate Radius into existing workflows and to migrate the first applications.
- **Incident response.** Developers own service-level failures while Platform Engineers own platform-level failures, but real incidents often pull in both, along with Operators and SREs, to diagnose root cause together.

The boundaries between all of these personas form a spectrum, not a set of walls. Designing for the handoffs and the shared middle ground, rather than for isolated audiences, is what makes the platform feel cohesive to everyone who uses it.
