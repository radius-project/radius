## How to run UCP on your local dev machine?

<br/><br/>

### Configuration

The local development configuration for UCP is in cmd/ucpd/ucp-self-hosted-dev.yaml. This is configured with default values for the different planes and is expected to work out of the box.

<br/><br/>

## Running UCP on your local dev machine

You can run cmd/ucp/main.go in vscode with the following configuration set in your .vscode/launch.json file:-
```
"configurations": [
    ....
        {
            "name": "Launch UCP",
            "type": "go",
            "request": "launch",
            "mode": "auto",
            "program": "${file}",
            "env": {
                "BASE_PATH": "/apis/api.ucp.dev/v1alpha3",
                "PORT": "9000",
                "UCP_CONFIG": "ucp-self-hosted-dev.yaml",
                "AWS_REGION": "{region}",
                "AWS_ACCESS_KEY_ID": "{aws key}",
                "AWS_SECRET_ACCESS_KEY": "{secret key}"
            },
            "args": [
            ]
        },
    ...
```

<br/><br/>

## Running UCP in a Kubernetes cluster

UCP is started as a part of the Radius installation. The helm charts and UCP configuration for deployment can be found under deploy/Chart/charts/ucp.

<br/><br/>

## Troubleshooting

* You can view UCP logs from the UCP container by using the command:-
```
kubectl logs {ucp pod name} -n radius-system -f
```
* You can directly send requests to UCP instead of going via the CLI to isolate issues while testing. For this, you could use any REST client like Postman with the URL such as:-
    ```
    URL:-
    PUT http://127.0.0.1:9000/apis/api.ucp.dev/v1alpha3/planes/radius/local/resourceGroups/{rg}/providers/Applications.Core/environments/{name}?api-version=2022-03-15-privatepreview

    Body:-
    {
        "location": "global",
        "properties": {
            "compute": {
                "namespace": "default",
                "kind": "kubernetes",
                "location": "global"
            }
        }
    }
    ```
