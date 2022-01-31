---
type: docs
title: "Create a local enviromnet and deploy the container microservices tutorial locally"
linkTitle: "Create Radius environment and deploy application microservices"
description: "Create Radius environment and deploy microservices"
weight: 2000
---

## Create the Radius environment

Open up a terminal and get ready to interact with the Radius Cli.

1. Create your Radius environment via the rad CLI:

   ```sh
   rad env init dev -i 
   ```

   This will create Docker containers containing local Kubernetes instances and ask you for a custom name for your local Radius environment. For confirmation you can check your '.rad/config.yaml' file which should contain your new environment settings.

   For this tutorial the environment name will be 'microservice-testing'.

   Your '.rad/config.yaml' file should look like this:

   ```yaml
   environment:
      default: microservice-testing
      items:
         microservice-testing:
            clustername: radius-microservice-testing
            context: k3d-radius-microservice-testing
            kind: dev
            namespace: default
            registry:
               pushendpoint: localhost:50522
               pullendpoint: radius-microservice-testing-registry:5000
   ```

1. Confirm that your Radius local Kubernetes environment is ready:

   ```sh
   rad env status
   ```

   You should see a table outputed with information regarding `Nodes`, `Registry`, `Ingress(HTTP)`, `Ingress(HTTPS)` of your enviroment. Example output:

   ```sh
      NODES        REGISTRY         INGRESS (HTTP)          INGRESS (HTTPS)
      Ready (2/2)  localhost:50522  http://localhost:50526  https://localhost:50525
   ```

1. Initialize Dapr for Kubernetes:

   ```sh
   dapr init -k
   ```

1. Run your Radius application:

   ```sh
   rad app run --profile microservice-testing 
   ```
   
   The profile parameter links your command to the specific environment you want to use. This should start up a process where your Bicep code is read and processed in order to start up each micro service.
