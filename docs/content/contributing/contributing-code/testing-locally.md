---
type: docs
title: "Test Radius locally"
linkTitle: "Test locally"
description: "How to run integration tests on Radius locally"
weight: 30
---

# Testing locally

Testing the RP locally can be challenging because the Radius RP is just one part of a distributed system. The actual processing of ARM templates (the output of a `.bicep file`) is handled by the ARM deployment engine, not us.

For this reason `rad` understands a special kind of environment called `localrp`. This emulates some of the **basic** features of ARM templates in `rad` so that you can test without the central ARM infrastructure.

## Pattern for integration testing

As a general pattern, you can find example applications in the `/examples` folder. Each folder has a `template.bicep` file which contains a deployable application.

If you are building new features, or want to test deployment interactions the best way is to either:

- Make a series of deploy and delete operations with one of these example applications
- Write a new example application

## Local testing with rad

You can use your build of `rad` (or build from source) to test against a local copy of the RP by creating a special environment.

To do this, open your environment file (`$HOME/.rad/config.yaml`) and edit it manually. 

You'll need to:

- Duplicate the contents of an Azure Cloud environment
- Give the new environment a memorable name like `test` or `local`
- Change the environment kind from `azure` to `localrp`
- Add a `url` property with the URL of your local RP

**Before**

```yaml
environment:
  default: my-cool-env
  items:
    my-cool-env:
      clustername: radius-aks-j5oqzddqmf36s
      kind: azure
      resourcegroup: my-cool-env
      controlplaneresourcegroup: RE-my-cool-env
      subscriptionid: 66d1209e-1382-45d3-99bb-650e6bf63fc0
```

**After**

```yaml
environment:
  default: my-cool-env
  items:
    local:
      clustername: radius-aks-j5oqzddqmf36s
      kind: localrp # remember to set the kind
      url: http://localhost:5000 # use whatever port you prefer when running the RP locally
      resourcegroup: my-cool-env
      controlplaneresourcegroup: RE-my-cool-env
      subscriptionid: 66d1209e-1382-45d3-99bb-650e6bf63fc0
    my-cool-env:
      clustername: radius-aks-j5oqzddqmf36s
      kind: azure
      resourcegroup: my-cool-env
      controlplaneresourcegroup: RE-my-cool-env
      subscriptionid: 66d1209e-1382-45d3-99bb-650e6bf63fc0
```

Now you can run `rad env switch local` and use this environment just like you'd use any other.

## Known Limitations

Since we're simulating the role of centralized ARM features like deployment templates there are some inherent limitations.

- Deploying Azure resources with `.bicep` is not supported
- Using `.bicep` constructs like parameters and variables is not supported