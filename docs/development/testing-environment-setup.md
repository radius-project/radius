# Testing environment setup

The `rad` CLI usually gets the deployment template from an online location. This isn't handy for developing the deployment template so we also support an override to pass in a file at the command line.

Example:

```sh
rad env init azure -i --deployment-template "./deploy/rp-full.json"
```