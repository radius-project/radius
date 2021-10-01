---
type: docs
title: "eShop on containers tutorial"
linkTitle: "eShop"
description: "Learn how to model and deploy a production-level application with Radius"
weight: 300
no_list: true
---

# Deploying eShop to Azure with Project Radius

[Project Radius](https://radapp.dev) is a new application and deployment model for applications across cloud and edge. This guide will detail how to deploy the eShop application using Radius.

## Pre-requisites

- [Azure CLI](https://docs.microsoft.com/en-us/cli/azure/install-azure-cli?view=azure-cli-latest)
- [rad CLI](https://radapp.dev/getting-started/install-cli)

## Deploy to Azure

1. First make sure you are logged into Azure:
   ```bash
   az login
   ```
1. Next initialize a Radius environment in Azure:
   ```bash
   rad env init azure -i
   ```
1. Now, deploy the eShop application to Azure:
   ```bash
   rad deploy ./eshop.bicep
   ```
1. Get the URL of the deployed application and visit the eShop:
   ```bash
   rad component show catalog -a eshop
   ```
1. Done!
