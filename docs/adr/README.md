# Architecture Design Records

This directory is where we store our Architecture Design Records (ADRs). If you're new to the team or would like to better understand the decision to use ADRs take a look at our first ADR [`000-adr-template.md`](000-adr-template.md) to build the necessary context.

## Directory Structure

We use the following directory structure:

* Use a folder per component
* Use a three digit prefix for easy file browsing

For example,
```
/docs/adr/ucp
/docs/adr/bicep-extensibility
/docs/adr/cli
...
````

File-naming
```
000-ucp-<ADR-Title>.md
```

## When should I write an ADR?

Joseph Blake, a Spotify engineer published: [When Should I Write an Architecture Decision Record](https://engineering.atspotify.com/2020/04/when-should-i-write-an-architecture-decision-record/) with an easy to follow instructions helping us to decide if an ADR should be written to record a decision.

The process is summarized nicely in the following diagram:
![adr-diagram](https://engineering.atspotify.com/wp-content/uploads/sites/2/2020/04/6b4d58b6-architecture-decision-record_diagram.png)