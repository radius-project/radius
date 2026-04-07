# Topic: GitHub App Graph Visualization Feature Spec

* **Author**: Will Tsai (@willtsai)

## Topic Summary

This feature brings Radius Application Graph visualization directly into the GitHub developer workflow. When a developer opens a pull request that modifies the source code and/or app definitions of a Radius application, a visualization of the application graph is rendered directly in the PR with a before/after diff visualization highlighting which resources and connections were added, removed, or changed. Additionally, on every merge to `main`, the updated app graph is rendered in the repository README, ensuring the architecture diagram in the repo root is always up to date and serves as a living architecture reference.

## User profile and challenges

### User persona(s)

- **Primary: Application Developer** — A developer who defines and modifies Radius applications. They work in a team, submit pull requests for code review, and want to understand how their application code and infrastructure changes affect the overall application topology.
- **Secondary: Platform Engineer / PR Reviewer** — An engineer who reviews pull requests and needs to quickly assess the scope and impact of application changes.
- **Tertiary: Team Lead / New Team Member** — Someone who looks at the repository README to understand the current application architecture at a glance.

### Challenge(s) faced by the user

1. **Blind code reviews for code and infrastructure changes**: When a developer modifies the application code or definitions, reviewers see raw code diffs but have no visual representation of how the application topology changed. Understanding the impact requires mentally parsing code changes, resource definitions, connection references, and cross-file dependencies.
2. **Stale architecture diagrams**: Teams often maintain architecture diagrams manually (e.g., in draw.io, Lucidchart, or static images). These diagrams drift out of date as the codebase evolves, leading to confusion and onboarding friction.
3. **No change-impact summary**: Developers cannot easily answer "what did my change break or affect?" without deploying the application and running `rad app graph`. This is too late in the development cycle.
4. **Context switching**: To see the application graph today, a developer must deploy the application to a cluster and run `rad app graph` from the CLI. This requires a running environment, which is not always available during code review.

### Positive user outcome

Developers and reviewers can see a clear, auto-generated visual diff of the application graph directly in the GitHub pull request, enabling faster and more confident code reviews. The repository README always contains an accurate, up-to-date architecture diagram, reducing onboarding time and eliminating stale documentation. Teams gain shift-left visibility into application topology changes without needing a running environment.

## Key scenarios

### Scenario 1: PR diff visualization with change highlighting

A developer modifies the app code and definition to add a new Redis cache and connect it to an existing container. When they open a pull request, the changes are visualized directly in the PR as an application graph with added resources/connections highlighted in green, removed ones in red, and unchanged ones in their default style.

### Scenario 2: README architecture diagram auto-update on merge

When a pull request that modifies the app code and/or definition is merged to `main`, the application graph diagram in the README is automatically regenerated to reflect the latest architecture. The README always shows the current state of the application, eliminating stale diagrams.

## Key dependencies and risks

TBD

## Key assumptions to test and questions to answer

TBD

## Current state

Radius has an existing Application Graph feature that operates at runtime:

- **CLI**: `rad app graph` command (`pkg/cli/cmd/app/graph/`) renders a text-based application graph showing resources, connections, and output resources.
- **API**: The `getGraph` action on `Applications.Core/applications` returns an `ApplicationGraphResponse` containing resources, connections (inbound/outbound), and output resources. Defined in `typespec/Applications.Core/applications.tsp`.
- **Backend**: Graph computation in `pkg/corerp/frontend/controller/applications/graph_util.go` uses breadth-first traversal over live API data to build a bidirectional adjacency map of application resources and their connections.
- **Dashboard**: The Radius Dashboard (separate repository) provides an interactive, zoomable graph visualization.
- **Bicep tooling**: The `bicep-tools/` directory contains a Bicep manifest parser and converter that may serve as a foundation for static Bicep analysis.

There is currently no static analysis capability for generating the application graph from Bicep source files, and no GitHub integration for graph visualization in PRs or README auto-update.

## Detailed user experience

### GitHub pull request workflow

#### Code changes and PR creation (for reference only, out of scope for this feature spec)

> Note that the features in this section are to illustrate the user experience following code changes and pull request creation. We use a Copilot-assisted scenario that will leverage some Radius AI Agent tooling (Skill, MCP Server, Platform Constitution) that are outside of the scope of this feature spec.

The user navigates to Copilot chat, points the context to their application repo, and prompts Copilot to make changes to their application. Leveraging the Radius app assembly tools (Skill, MCP Server, Platform Constitution), Copilot plans to implement the app code to use Radius features like Connections and also knows to update the app definition. It proposes a plan for the changes and asks user to confirm the implementation:
![alt text](2026-04-github-app-graph-visualization-feature-spec/image01.png)

The user accepts the proposed plan and changes from Copilot and prompts it to proceed:
![alt text](2026-04-github-app-graph-visualization-feature-spec/image02.png)

Copilot proceeds to make the code changes and creates a PR in the app repo:
![alt text](2026-04-github-app-graph-visualization-feature-spec/image03.png)

#### Radius auto-generates app graph visualizations

In the PR view, the Radius UI component (e.g. browser extension) detects that the PR includes updates to the app.bicep file and begins to auto generate an app graph for visualization:
![alt text](2026-04-github-app-graph-visualization-feature-spec/image04.png)

Radius renders the app graph visualization that shows the diff of added (green), modified (yellow), and removed (red) components. The visualization is inserted into the body of the PR description directly below the PR description text:
![alt text](2026-04-github-app-graph-visualization-feature-spec/image05.png)

#### Interactions with diff views and app graph visualizations

The app graph visualization is an interactive UI component that the user may click to navigate to relevant parts of the repo for each component. Since this is the abstract app graph as-modeled (i.e. not yet deployed), clicking into each component (e.g. “cache”) gives the user options to navigate to relevant parts of either (1) the app source code or (2) the app definition file:
![alt text](2026-04-github-app-graph-visualization-feature-spec/image06.png)

**NOTE:** The pointer to the source code file for each resource will be provided by the author (either human or AI) of the app definition in the `app.bicep` file. This means that a new optional property called `source` (or equivalent) will need to be added to each resource schema. If the `source` property is not provided by the author, the app graph visualization will still render but without the option to navigate to the source code for that component. The `source` property will be a string that contains the file path relative to the repository root, for example:
```bicep
resource redisCache 'Applications.Datastores/redisCaches@2023-10-01-preview' = {
  name: 'redisCache'
  properties: {
    application: radiustodoapp.id
    environment: environment
    // New property indicating source code location for this resource
    //  can also be './src/cache/redis.ts#L10' for line number ref
    source: './src/cache/redis.ts'
  }
}
```

When the user clicks on the source code hyperlink for a _modified_ component (e.g. “cache”), they are taken to the diff view in the PR for the _modified_ source code:
![alt text](2026-04-github-app-graph-visualization-feature-spec/image07.png)

When the user clicks on the app definition hyperlink for a _modified_ component (e.g. “cache”), they are taken to the diff view in the PR for the relevant line of the _modified_ app definition (e.g. line #71):
![alt text](2026-04-github-app-graph-visualization-feature-spec/image08.png)

Clicking into an _unchanged_ component (e.g. “database”) also gives the user options to navigate to relevant parts of either (1) the app source code or (2) the app definition file:
![alt text](2026-04-github-app-graph-visualization-feature-spec/image09.png)

When the user clicks on the source code hyperlink for an _unchanged_ component (e.g. “database”), they are taken to the GitHub page for the relevant source code:
![alt text](2026-04-github-app-graph-visualization-feature-spec/image10.png)

#### Merge the PR into `main` branch

The user navigates to the README in the repo root on the `main` branch and notices that the architecture diagram previously generated by Radius does not yet reflect the changes from their not-yet-merged PR:
![alt text](2026-04-github-app-graph-visualization-feature-spec/image11.png)

The user navigates back to their PR and clicks on “Merge pull request” and “Confirm merge” in the PR to merge in the code changes:
![alt text](2026-04-github-app-graph-visualization-feature-spec/image12.png)

#### Radius-generated architecture diagram is updated in the README

The merge into the `main` branch triggers the Radius UI component to generate an updated app graph diagram under the “Architecture” section in the README to reflect the recently merged application code changes. Note that the app graph visualization no longer includes the red, yellow, green coloring from the PR view:
![alt text](2026-04-github-app-graph-visualization-feature-spec/image13.png)

Just like in the PR views, the app graph visualization is an interactive UI component that the user may click to navigate into the relevant parts of the code in the repo on GitHub:
![alt text](2026-04-github-app-graph-visualization-feature-spec/image14.png)

## Key investments

TBD
