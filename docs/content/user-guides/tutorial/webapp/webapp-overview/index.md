---
type: docs
title: "Website tutorial app overview"
linkTitle: "App overview"
description: "Overview of the website tutorial application"
weight: 1000
---

You will be deploying a *To-Do List* website. It will have three Radius *components*:

1. **todoapp**: A containerized to-do application written in Node.JS
2. **db**: A MongoDB database to save to-do items in
3. **kv**: An Azure KeyVault for secret management (Azure environments only)

<img src="todoapp-diagram.png" width=400 alt="Simple app diagram">

## Website frontend

The example website (`todoapp`) is a single-page-application (SPA) with a Node.JS backend. The SPA sends requests HTTP requests to the Node.JS backend to read and store *todo* items.

The website listens on port 3000 for HTTP requests. 

The website uses the MongoDB protocol to read and store data in a database. The website reads the environment variable `DBCONNECTION` to discover the database connection string.

#### (optional) Download the source code

You can download the source code [here](/tutorial/webapp.zip) if you want to see how the frontend is built.

## Database

The database (`db`) is an Azure Cosmos MongoDB database.

## The Radius mindset

The diagrams shown so far document the communication flows, but a Radius application also describes additional details. 

A Radius template includes 

- The logical relationships of an application 
- The operational details associated with those relationships 

Here is an updated diagram that shows what the Radius template captures:

<img src="todoapp-appdiagram.png" width=800 alt="App diagram with descriptions of all the details and relationships."><br />

### Benefits

- Support for data components (`db`) are part of the application
- Relationships between components are fully specified with protocols and other strongly-typed information

In addition to this high level information, the Radius model also uses typical details like:

- Container images
- Listening ports
- Configuration like connection strings

Keep the diagram in mind as you proceed through the following steps. Your Radius deployment template will aim to match it. 


<br>{{< button text="Next: Deploy the website frontend" page="webapp-initial-deployment.md" >}}

