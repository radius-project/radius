---
type: docs
title: "Troubleshooting Radius issues and common problems"
linkTitle: "Troubleshooting"
description: "Common issues users may have with Radius and how to address them"
weight: 35
---

## Cloning repo

### Visual Studio not authorized for single sign-on

If you receive an error saying Visual Studio or another application is not authorized to clone the Radius repo and you need to re-authorize the app, follow these steps:
1. Open a browser to https://github.com/Azure/radius
1. Select your profile and click on Settings
1. Select Applications from the left navbar
1. Select the Authorized OAuth Apps tab
1. Find the conflicting app and select Revoke
1. Reopen app on local machine and re-auth


## Creating environment

## Error response cannot be parsed: """ error: EOF

If you get an error when initializing an Azure environment after selecting a Resource Group name, make sure you set your subscription to one within the Microsoft tenant:

```bash
az account set --subscription <SUB-ID>
```

## Doskey is not recognized

If you receive an error about the `doskey` binary, such as below:

```bash
Invoking Azure CLI failed with the following error: 'doskey' is not recognized as an internal or external command, operable program or batch file.
```

try some of these solutions:
- Make sure C:\Windows\System32 is part of your PATH
- Make sure you don't have any custom scripts that launch at startup that use doskey to configure special aliases

## Application deployment

Issues faced when deploying applications

