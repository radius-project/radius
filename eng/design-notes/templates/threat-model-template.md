# Title

- **Author**: Your name (@YourGitHubUserName)

## Overview

<!--
Provide a succinct high-level description of the system, feature, or component being analyzed. Explain why a threat model is being created for this system, feature, or component. This section should be one to three paragraphs long and understandable by someone outside the Radius team.

Provide links to supporting documentation like feature specs and design documents rather than duplicating information.
-->

## Terms and Definitions

<!--
Include any terms, definitions, or acronyms that are used in this threat model document to assist the reader. They may or may not be part of the user-facing experience once implemented, and can be specific to this design context.
-->

## System Description

<!--
Provide a detailed description of the system or feature being modeled. Include information key components, and interactions with other systems.
-->

### Architecture

<!-- Overview of the system architecture of the component that is being discussed in this document. -->

### Implementation Details

<!-- What are the components of the implementation -->

**Is there any use of cryptography?**

<!-- Answer YES/NO and, if yes, please describe (the type of the cryptography used, their purpose, and libraries used) -->

<!-- Examples can include encryption and hashing. -->

**Does the component store secrets?**

<!-- Answer YES/NO and, if yes, please describe the type of data and how it is stored. -->

**Does the component process untrusted data or does the component parse any custom formats?**

<!-- Answer YES/NO and, if yes, please describe the type of data and the libraries that are used to parse the data. -->

<!-- Ex: data coming from a user. -->

### Clients

<!-- Clients that communicate with the component that is being reviewed in the threat model. -->

## Trust Boundaries

<!-- Define the limits within which components can interact without additional security checks. They help identify where security controls are needed to protect against threats. -->

## Assumptions

<!-- Outline the conditions presumed to be true for the threat model. These assumptions set the context and scope, highlighting what is considered secure and what is not evaluated. -->

## Data Flows

<!--
Include a diagram of the system architecture, showing how different components interact. Highlight any areas where security controls are implemented or where threats might be present.
-->

### Diagram

<!-- The diagram for the threat model. It can be done by using Microsoft Threat Modeling Tool. -->

## Threats

<!--

Use this section to list possible security threats.

For an primer on types of threats please see: https://en.wikipedia.org/wiki/STRIDE_model

Good threats are specific to the design and implementation of the system.

Good: `A malicious user could spoof the 'user id' field and request another user's data leading to unauthorized information disclosure.`

Bad: `If we have a bug, a user might see data they are not authorized to see.`

For each threat copy-paste and fill-out the template below. DO NOT omit fields if you are unsure of the answers.

-->

### Threat 1: Threat about a component

**Description:** <!-- Provide a clear and specific description of the threat, including any malicious actions or system conditions that would cause a vulnerability. -->
**Impact:** <!-- Provide a clear and specific description of the impact if this threat were exploited. -->
**Mitigation:** <!-- Describe the existing or possible mitigations in place for this threat. -->
**Status:** <!-- Describe the status of each mitigation. Is this mitigation already in place (active or planned)? If this mitigation on-by-default or does it require setup by the user?  -->

<!--

And example threat is talking about two servers: Server A and Server B. These servers talk to each other to trigger some actions.

### Threat 1: Spoofing Server A could cause information disclosure

**Description:** An attacker can spoof Server A by tampering with the configuration in the Server B. Server B will start sending requests to the fake Server A which will cause information disclosure.

**Impact:** All data that should be sent to the Server A by Server B will be available to the fake Server A including payloads, headers, and other sensitive information.

**Mitigation:**

1. Regularly rotate and manage server credentials.
2. Use Role-Based Access Control (RBAC) to limit permissions and enforce the principle of least privilege.
3. Monitor and audit API server access logs for suspicious activities.

**Status:**

- Credential rotation and management: Active
- RBAC implementation: Active
- Monitoring and auditing: Active

-->

## Open Questions

<!--
List any unresolved questions or uncertainties about the threat model. Use this section to gather feedback from experts or team members and to track decisions made during the review process.
-->

## Action Items

<!--
The list of action items that will be done in order to improve the safety of the system.
-->

## Review Notes

<!--
Update this section with the decisions and feedback from the threat model review meeting. Document any changes made to the model based on the review.
-->
