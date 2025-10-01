# Radius Git HTTP Backend

This directory contains a lightweight Git HTTP backend used by the Kubernetes
functional tests. The binary wraps the upstream `githttpbackend`
executable, exposes it over HTTP with optional basic authentication, and can
seed a bare repository on startup.

## Building

Use the existing build system to create the multi-platform binaries and image:

```
make build-githttpbackend-linux-amd64
make docker-build-githttpbackend
```

Both commands honour `DOCKER_REGISTRY` and `DOCKER_TAG_VERSION`, matching the
other test utilities. You can also build the container directly:

```
docker build -f test/githttpbackend/Dockerfile -t radius-githttpbackend:test .
```

## Runtime configuration

The container accepts the following environment variables:

| Variable | Description | Default |
| --- | --- | --- |
| `GIT_HTTP_PORT` | Port to listen on. | `3000` |
| `GIT_SERVER_TEMP_DIR` | Root directory that hosts bare repositories. | `/var/lib/git` |
| `GIT_USERNAME` | Username for HTTP basic authentication. | _required_ |
| `GIT_PASSWORD` | Password for HTTP basic authentication. | _required_ |
| `GIT_REPO_NAME` | Optional repository name to initialize as a bare repo on start. | _unset_ |

At startup the binary enables `http.receivepack` globally so pushes are
permitted. When `GIT_REPO_NAME` is set, it creates `<GIT_REPO_NAME>.git`
inside the repo root if the repository does not already exist.

## Usage

```
docker run \
  -e GIT_USERNAME=testuser \
  -e GIT_PASSWORD=testpass \
  -e GIT_REPO_NAME=demo \
  -p 3000:3000 \
  radius-githttpbackend:test
```

Clients can push to `http://localhost:3000/demo.git` using the configured
credentials.
