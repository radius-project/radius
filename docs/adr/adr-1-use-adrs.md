# ADR 1: Use Architectural Decision Records (ADRs)

## Status

accepted

## Context

An architecture decision record is a short text file in a format. Each record describes a set of considerations that led to a significant decision in the process of creating a software system, the decisions made and the consequences of those decisions.

ADRs were initially proposed by Michael Nygard in his article ["Documenting Architecture Decisions"](https://www.cognitect.com/blog/2011/11/15/documenting-architecture-decisions), in the article the proposed format and a description of each one of the sections that compose it is detailed.

The ADR document aims to contain just enough information to help individuals onboarding or reviewing the project to understand how the team arrived at the current solution and what were the driving forces that shaped this current state.

Notably, Michael Nygard mentions:

>The whole document should be one or two pages long using the same layout this document follows. We will write each ADR as if it is a conversation with a future  developer. This requires good writing style, with full sentences organized into paragraphs. Bullets are acceptable only for visual style, not as an excuse for  writing sentence fragments.
(Bullets kill people, even PowerPoint bullets.)

To ease the process of creation of ADRs we recommend the usage of the console application [adr-tools](https://github.com/npryce/adr-tools), however its use is not required to create ADRs.

### When should I write an ADR?

Joseph Blake, a Spotify engineer published: [When Should I Write an Architecture Decision Record](https://engineering.atspotify.com/2020/04/when-should-i-write-an-architecture-decision-record/) with an easy to follow instructions helping us to decide if an ADR should be written to record a decision.

The process is summarized nicely in the following diagram:
![adr-diagram](https://engineering.atspotify.com/wp-content/uploads/sites/2/2020/04/6b4d58b6-architecture-decision-record_diagram.png)

## Decision

We will adopt the use of ADRs to record "architecturally significant" decisions.
ADRs will be review as Pull Requests to our repository and kept as Markdown files in the docs/adr directory. The ADRs will follow the format proposed by Michael Nygard and documented as a template adr in [`adr-0-template.md`](adr-0-template.md)

The following process will be used to approve ADRs:

  1. The author will use the decision diagram (described above) to decide if an ADR should be added with their code submission.
  1. The author will submitted the ADR and create a pull request to this repository alongside the implementation (when applicable)
  1. The author will identify the people required to accept the decision request their review using the standard GitHub process.
  1. If an ADR is meant to supersede a previously approved ADR, the author will make a change on the superseded documents as part of their ADR pull request submission.

## Consequences

* Future team members are able to read a history of decisions and quickly get up to speed on how and why a decision is made, and the impact of that decision
* ADRs enable our teams to align on best practices across our project and will enhance the consistency and cohesion of our codebase
* The process of discussing "architecturally significant" decisions will be more inclusive to team members and enable relevant people to offer their thoughts and opinions in an asynchronous way
* Using ADRs will require authors to make a significant time investment articulating clearly the decisions being made and the alternatives considered.
* It will take some time to familiarize and adopt correctly ADRs and some churn should be expected as the team refines the process and accommodates to the new paradigm
