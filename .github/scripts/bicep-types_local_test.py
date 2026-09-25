# Copyright 2026 The Radius Authors.
# Licensed under the Apache License, Version 2.0.

"""Local-only native Bicep packaging, exact copy, retry, restore and compile check."""

import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import time
from unittest.mock import patch
from urllib.request import urlopen

spec = importlib.util.spec_from_file_location(
    "bicep_types", Path(__file__).with_name("bicep-types.py")
)
publisher = importlib.util.module_from_spec(spec)
spec.loader.exec_module(publisher)
IMAGE = (
    "registry@sha256:a3d8aaa63ed8681a604f1dea0aa03f100d5895b6a58ace528858a7b332415373"
)


def run(*args):
    return subprocess.check_output(args, timeout=60).decode().strip()


def main():
    run("docker", "image", "inspect", IMAGE)
    containers = []
    try:
        endpoints = []
        for _ in range(2):
            container = run(
                "docker", "run", "--detach", "--publish", "127.0.0.1::5000", IMAGE
            )
            containers.append(container)
            port = run("docker", "port", container, "5000/tcp").rsplit(":", 1)[1]
            for attempt in range(20):
                try:
                    with urlopen(f"http://127.0.0.1:{port}/v2/", timeout=1) as response:
                        assert response.status == 200
                    break
                except OSError:
                    if attempt == 19:
                        raise
                    time.sleep(0.2)
            endpoints.append(f"localhost:{port}")
        with tempfile.TemporaryDirectory(prefix="radius-bicep-publishing-") as scratch:
            scratch = Path(scratch)
            source = {
                "repository": publisher.REPOSITORY,
                "ref": publisher.REF,
                "commit": run("git", "rev-parse", "HEAD"),
                "workflow": publisher.WORKFLOW,
                "runId": "1",
                "generationAttempt": 1,
            }
            publisher.write_json(scratch / "source.json", source)
            publisher.write_json(scratch / "docker/config.json", {})
            with patch.dict(
                os.environ,
                {
                    "DOCKER_CONFIG": str(scratch / "docker"),
                    "BICEP_TRUSTED_REGISTRIES": "localhost",
                    "BICEP_CACHE_ROOT_DIRECTORY": str(scratch / "cache"),
                },
            ):
                publisher.capture(
                    scratch / "source.json",
                    publisher.ROOT / "hack/bicep-types-radius/generated/index.json",
                    endpoints[0],
                    scratch / "bundle",
                )
                metadata, native = publisher.validate_bundle(scratch / "bundle", source)
                final_digest = publisher.stamp(
                    scratch / "bundle", metadata, native, scratch / "final"
                )
                retry_digest = publisher.stamp(
                    scratch / "bundle", metadata, native, scratch / "retry"
                )
                assert final_digest == retry_digest
                # Only ORAS copies below: neither generation nor Bicep packaging is repeated.
                for _ in range(2):
                    publisher.copy_exact(
                        f"{scratch / 'final'}@{final_digest}",
                        f"{endpoints[0]}/radius:edge",
                        final_digest,
                        from_layout=True,
                        plain_http=True,
                    )
                    publisher.copy_exact(
                        f"{endpoints[0]}/radius@{final_digest}",
                        f"{endpoints[1]}/radius:latest",
                        final_digest,
                        plain_http=True,
                    )
                for endpoint, tag in zip(endpoints, ["edge", "latest"]):
                    assert (
                        publisher.verify_manifest(
                            f"{endpoint}/radius:{tag}", final_digest, True
                        )
                        == (
                            scratch / "final/blobs/sha256" / final_digest.split(":")[1]
                        ).read_bytes()
                    )
                newer_metadata = {**metadata, "source": {**source, "commit": "b" * 40}}
                newer_digest = publisher.stamp(
                    scratch / "bundle", newer_metadata, native, scratch / "newer"
                )
                publisher.copy_exact(
                    f"{scratch / 'newer'}@{newer_digest}",
                    f"{endpoints[0]}/radius:edge",
                    newer_digest,
                    from_layout=True,
                    plain_http=True,
                )
                compiled_metadata = []
                for endpoint, tag in zip(endpoints, ["edge", "latest"]):
                    consumer = scratch / tag
                    consumer.mkdir()
                    publisher.write_json(
                        consumer / "bicepconfig.json",
                        {
                            "experimentalFeaturesEnabled": {"ociEnabled": True},
                            "cacheRootDirectory": str(consumer / "cache"),
                            "extensions": {
                                "radius": f"br:{endpoint}/radius@{final_digest}"
                            },
                        },
                    )
                    (consumer / "application.bicep").write_text(
                        "extension radius\nresource app 'Radius.Core/applications@2025-08-01-preview' = {\n"
                        "  name: 'publisher-test'\n  location: 'global'\n  properties: {\n"
                        "    environment: 'test'\n  }\n}\noutput appId string = app.id\n"
                    )
                    publisher.command(
                        "bicep",
                        "restore",
                        str(consumer / "application.bicep"),
                        cwd=consumer,
                    )
                    publisher.command(
                        "bicep",
                        "build",
                        str(consumer / "application.bicep"),
                        cwd=consumer,
                    )
                    compiled = publisher.read_json(consumer / "application.json")
                    assert "br:localhost" not in json.dumps(compiled)
                    compiled_metadata.append(
                        {
                            "imports": compiled.get("imports"),
                            "extensions": compiled.get("extensions"),
                        }
                    )
                print(
                    json.dumps(
                        {
                            "nativeIndexDescriptor": publisher.read_json(
                                scratch / "bundle/layout/index.json"
                            )["manifests"][0],
                            "nativeManifest": native,
                            "finalDigest": final_digest,
                            "retryDigest": retry_digest,
                            "newerAliasDigest": newer_digest,
                            "compiledExtensionMetadata": compiled_metadata,
                            "digestCopyAndAnonymousCompile": "passed",
                        },
                        indent=2,
                    )
                )
    finally:
        for container in containers:
            subprocess.run(
                ["docker", "rm", "--force", container], check=True, timeout=30
            )


if __name__ == "__main__":
    main()
