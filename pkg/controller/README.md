# Threat Modeling for the Controller Component

## Process

- Explain the feature/component at a high level
- Data flows
- Trust boundaries
- Data serialization/formats
- Use of crypto
- Storage of secrets

## Controller Component Threat Model

### Controller Component Explained

Controller creates a new Manager registers the following with it:

- Two controllers: Recipe and Deployment reconcilers. Each controller is responsible for watching specific Kubernetes resources and reconciling their state.
- A Webhook if TLS certificates are present.

### Clients

- Kubernetes API Server: The primary client that interacts with the controller. It sends events related to resource changes (e.g., creation, update, deletion) to the controller manager. The controller watches for these events and reconciles the state of the resources accordingly.
- Webhook Clients: If webhooks are enabled and registered, clients that interact with the Kubernetes API server (e.g., `kubectl`, other controllers) can trigger webhook calls.
- Health Check Probes: Kubernetes itself can act as a client by performing health and readiness checks on the controller manager.
- Metrics Scrapers: If metrics are enabled, Prometheus or other monitoring tools can scrape metrics from the controller manager.

### Interaction with Clients

#### Kubernetes API Server

The communication between the Controller component and the Kubernetes API Server is a two-way interaction.

1. The Kubernetes API Server sends events (e.g., resource creation, updates, deletions) to the controller manager. This is done through the Kubernetes API, which the controller watches for changes.
1. The controller makes API calls to the Kubernetes API server to fetch the current state of resources, update resource statuses, and perform other operations like creating, updating, or deleting resources.

The communication protocol is always over HTTPS (HTTP over TLS). This ensures that the data exchanged between the controller and the API server is encrypted and secure.

Another type of communication between the Kubernetes API Server and the Controller component happens through the webhook server.

1. The Kubernetes API Server intercepts the request of create, update, or delete of a Recipe and sends it to the registered validating webhook for validation. The webhook URL is configured to point to the controller's webhook server.
1. The webhook server receives and processes the request, and performs validation logic on the resource.
1. Based on the validation result, the webhook server responds to the Kubernetes API Server with either an approval or rejection of the request.
