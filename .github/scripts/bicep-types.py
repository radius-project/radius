# Copyright 2026 The Radius Authors.
# Licensed under the Apache License, Version 2.0.

"""Capture and publish the Radius main Bicep types without executing bundle content."""

import argparse
import hashlib
import io
import json
import os
from pathlib import Path, PurePosixPath
import re
import shutil
import stat
import subprocess
import sys
import tarfile
import tempfile
import time
from urllib.error import HTTPError, URLError
from urllib.parse import urlparse
from urllib.request import HTTPRedirectHandler, Request, build_opener, urlopen
import zipfile

REPOSITORY = "radius-project/radius"
WORKFLOW = ".github/workflows/build-main.yaml"
REF = "refs/heads/main"
GHCR = "ghcr.io/radius-project/bicep-types-radius:edge"
ACR = "biceptypes.azurecr.io/radius:latest"
MANIFEST_TYPE = "application/vnd.oci.image.manifest.v1+json"
ARTIFACT_TYPE = "application/vnd.ms.bicep.provider.artifact"
CONFIG_TYPE = "application/vnd.ms.bicep.provider.config.v1+json"
LAYER_TYPE = "application/vnd.ms.bicep.provider.layer.v1.tar+gzip"
MAX_BYTES = 64 * 1024 * 1024
MAX_EXPANDED_BYTES = 128 * 1024 * 1024
MAX_FILES = 2000
ROOT = Path(__file__).resolve().parents[2]


def require(condition, message):
    if not condition:
        raise ValueError(message)


def digest(data):
    return "sha256:" + hashlib.sha256(data).hexdigest()


def valid_digest(value):
    return isinstance(value, str) and re.fullmatch(r"sha256:[0-9a-f]{64}", value)


def positive(value):
    require(
        isinstance(value, str) and re.fullmatch(r"[1-9][0-9]*", value),
        "Expected a positive decimal identity",
    )
    number = int(value)
    require(number <= 2**53 - 1, "Identity exceeds the safe integer range")
    return number


def decode_json(data):
    def unique_fields(pairs):
        result = {}
        for key, value in pairs:
            require(key not in result, "Duplicate JSON field")
            result[key] = value
        return result

    def invalid_number(value):
        raise ValueError(f"Invalid JSON number: {value}")

    return json.loads(
        data, object_pairs_hook=unique_fields, parse_constant=invalid_number
    )


def read_json(path):
    return decode_json(Path(path).read_bytes())


def write_json(path, value):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_suffix(path.suffix + ".tmp")
    temporary.write_text(
        json.dumps(value, sort_keys=True, separators=(",", ":")) + "\n"
    )
    temporary.replace(path)


def output(**values):
    if "GITHUB_OUTPUT" in os.environ:
        with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8") as stream:
            for key, value in values.items():
                require("\n" not in str(value), "Invalid workflow output")
                stream.write(f"{key}={value}\n")


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


class GitHub:
    def __init__(self):
        self.token = os.environ["GH_TOKEN"]
        require(bool(self.token), "GH_TOKEN is required")
        self.deadline = time.monotonic() + 120

    def _read(self, response, limit):
        data = bytearray()
        while True:
            require(time.monotonic() < self.deadline, "GitHub read deadline exceeded")
            chunk = response.read(min(65536, limit + 1 - len(data)))
            if not chunk:
                return bytes(data)
            data.extend(chunk)
            require(len(data) <= limit, "GitHub response exceeds the size limit")

    def get(self, path, archive=False):
        require(
            path.startswith(f"/repos/{REPOSITORY}/")
            or path == "/orgs/radius-project/packages/container/bicep-types-radius",
            "Unexpected GitHub API destination",
        )
        headers = {
            "Authorization": f"Bearer {self.token}",
            "Accept": "application/vnd.github+json",
            "X-GitHub-Api-Version": "2022-11-28",
        }
        request = Request("https://api.github.com" + path, headers=headers)
        for attempt in range(3):
            remaining = self.deadline - time.monotonic()
            require(remaining > 0, "GitHub API deadline exceeded")
            try:
                with build_opener(NoRedirect()).open(
                    request, timeout=min(20, remaining)
                ) as response:
                    if archive:
                        return self._read(response, MAX_BYTES)
                    return decode_json(self._read(response, 2 * 1024 * 1024))
            except HTTPError as error:
                if archive and error.code == 302:
                    location = error.headers["Location"]
                    parsed = urlparse(location)
                    require(
                        parsed.scheme == "https"
                        and parsed.hostname
                        and not parsed.username,
                        "Invalid artifact download redirect",
                    )
                    # Do not send the repository token to the signed artifact-storage URL.
                    with urlopen(location, timeout=min(20, remaining)) as response:
                        return self._read(response, MAX_BYTES)
                if error.code not in (429, 500, 502, 503, 504) or attempt == 2:
                    raise RuntimeError(
                        f"GitHub GET {path} failed with HTTP {error.code}"
                    ) from error
                delay = float(error.headers.get("Retry-After", 2**attempt))
                require(
                    0 <= delay < self.deadline - time.monotonic(),
                    "GitHub retry exceeds deadline",
                )
                time.sleep(delay)
        raise RuntimeError("GitHub retries exhausted")


def source_context(api):
    require(
        os.environ["GITHUB_REPOSITORY"] == REPOSITORY,
        "Only the canonical Radius repository may publish",
    )
    require(os.environ["GITHUB_REF"] == REF, "Only main may publish edge")
    require(
        os.environ["GITHUB_EVENT_NAME"] in ("push", "workflow_dispatch"),
        "Untrusted publishing event",
    )
    require(
        os.environ["BICEP_SOURCE_REF_PROTECTED"] == "true", "Protected main is required"
    )
    require(
        os.environ["GITHUB_WORKFLOW_REF"] == f"{REPOSITORY}/{WORKFLOW}@{REF}",
        "Only build-main may publish edge",
    )
    sha = os.environ["GITHUB_SHA"]
    require(
        re.fullmatch(r"[0-9a-f]{40}", sha) and sha != "0" * 40, "Invalid source commit"
    )
    run_id, attempt = positive(os.environ["GITHUB_RUN_ID"]), positive(
        os.environ["GITHUB_RUN_ATTEMPT"]
    )
    run = api.get(f"/repos/{REPOSITORY}/actions/runs/{run_id}")
    require(
        run["id"] == run_id
        and run["run_attempt"] == attempt
        and run["event"] == os.environ["GITHUB_EVENT_NAME"]
        and run["path"] == WORKFLOW
        and run["head_branch"] == "main"
        and run["head_sha"] == sha
        and run["repository"]["full_name"] == REPOSITORY
        and run["head_repository"]["full_name"] == REPOSITORY,
        "Source run identity mismatch",
    )
    return {
        "repository": REPOSITORY,
        "ref": REF,
        "commit": sha,
        "workflow": WORKFLOW,
        "runId": str(run_id),
        "generationAttempt": attempt,
    }


def bundle_name(source):
    return f"bicep-types-radius-{source['runId']}"


def receipt_name(source, attempt):
    return f"bicep-types-radius-receipt-{source['runId']}-{attempt}"


def artifacts(api, source):
    entries = []
    for page in range(1, 11):
        result = api.get(
            f"/repos/{REPOSITORY}/actions/runs/{source['runId']}/artifacts?per_page=100&page={page}"
        )
        total = result["total_count"]
        require(
            type(total) is int and 0 <= total <= 1000,
            "Artifact listing is invalid or truncated",
        )
        current = result["artifacts"]
        require(
            isinstance(current, list)
            and len(current) == min(100, max(0, total - (page - 1) * 100)),
            "Incomplete artifact listing",
        )
        entries.extend(current)
        if page * 100 >= total:
            return entries
    raise ValueError("Artifact pagination limit exceeded")


def validate_artifact(artifact, source, name):
    require(
        artifact["name"] == name and artifact["expired"] is False,
        "Mismatched or expired artifact",
    )
    require(type(artifact["id"]) is int and artifact["id"] > 0, "Invalid artifact ID")
    require(
        type(artifact["size_in_bytes"]) is int
        and 0 < artifact["size_in_bytes"] <= MAX_BYTES,
        "Invalid artifact size",
    )
    require(
        valid_digest(artifact["digest"]), "Missing or invalid Actions artifact digest"
    )
    run = artifact["workflow_run"]
    require(
        run["id"] == int(source["runId"])
        and run["head_branch"] == "main"
        and run["head_sha"] == source["commit"],
        "Artifact belongs to a different source run",
    )


def find_artifact(entries, source, name):
    matches = [item for item in entries if item["name"] == name]
    require(len(matches) <= 1, "Ambiguous artifact name")
    if not matches:
        return None
    validate_artifact(matches[0], source, name)
    return matches[0]


def select(api, directory):
    source = source_context(api)
    found = find_artifact(artifacts(api, source), source, bundle_name(source))
    if found:
        output(reuse="true", artifact_id=found["id"], artifact_digest=found["digest"])
    else:
        require(
            source["generationAttempt"] == 1,
            "Retry bundle is missing; start a new approved main run instead of regenerating",
        )
        write_json(Path(directory) / "source.json", source)
        output(reuse="false")


def safe_path(name):
    require(
        isinstance(name, str)
        and name
        and "\\" not in name
        and not name.startswith("/")
        and all(part not in ("", ".", "..") for part in name.split("/")),
        "Unsafe archive path",
    )
    return PurePosixPath(name)


def extract_artifact(api, artifact, destination):
    data = api.get(
        f"/repos/{REPOSITORY}/actions/artifacts/{artifact['id']}/zip", archive=True
    )
    require(digest(data) == artifact["digest"], "Actions archive digest mismatch")
    destination = Path(destination)
    require(not destination.exists(), "Artifact destination must be fresh")
    with zipfile.ZipFile(io.BytesIO(data)) as archive:
        entries = archive.infolist()
        require(0 < len(entries) <= MAX_FILES, "Invalid archive entry count")
        require(
            sum(item.file_size for item in entries) <= MAX_BYTES,
            "Expanded workflow artifact is too large",
        )
        seen = set()
        for entry in entries:
            name = entry.filename.rstrip("/") if entry.is_dir() else entry.filename
            safe_path(name)
            require(name not in seen, "Duplicate archive entry")
            seen.add(name)
            mode = entry.external_attr >> 16
            require(not stat.S_ISLNK(mode), "Artifact links are forbidden")
            require(
                not mode & 0o111 or entry.is_dir(),
                "Executable artifact entries are forbidden",
            )
            require(
                entry.is_dir() or stat.S_IFMT(mode) in (0, stat.S_IFREG),
                "Non-file artifact entry",
            )
        destination.mkdir(parents=True)
        for entry in entries:
            target = destination / entry.filename
            if entry.is_dir():
                target.mkdir(parents=True, exist_ok=True)
            else:
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_bytes(archive.read(entry))


def descriptor_blob(layout, descriptor, media_type):
    require(
        set(descriptor)
        <= {"mediaType", "digest", "size", "annotations", "artifactType"},
        "Unsupported descriptor fields, including foreign URLs",
    )
    require(
        descriptor["mediaType"] == media_type and valid_digest(descriptor["digest"]),
        "Invalid OCI descriptor type or digest",
    )
    require(
        type(descriptor["size"]) is int and 0 < descriptor["size"] <= MAX_BYTES,
        "Invalid OCI blob size",
    )
    path = layout / "blobs" / "sha256" / descriptor["digest"].split(":")[1]
    require(path.is_file() and not path.is_symlink(), "Missing OCI blob")
    data = path.read_bytes()
    require(
        len(data) == descriptor["size"] and digest(data) == descriptor["digest"],
        "OCI blob digest or size mismatch",
    )
    return data


def validate_types(data):
    with tarfile.open(fileobj=io.BytesIO(data), mode="r:gz") as archive:
        names, total = set(), 0
        index = None
        for entry in archive:
            safe_path(entry.name)
            require(
                entry.isfile()
                and entry.name.endswith(".json")
                and not entry.mode & 0o111,
                "Types archive must contain only non-executable JSON files",
            )
            require(entry.name not in names, "Duplicate type file")
            names.add(entry.name)
            total += entry.size
            require(
                len(names) <= MAX_FILES and total <= MAX_EXPANDED_BYTES,
                "Types archive exceeds limits",
            )
            value = decode_json(archive.extractfile(entry).read())
            if entry.name == "index.json":
                index = value
        require(
            isinstance(index, dict) and isinstance(index.get("resources"), dict),
            "Missing type index",
        )

        def check_refs(value):
            if isinstance(value, dict):
                for key, child in value.items():
                    if key == "$ref":
                        require(
                            isinstance(child, str) and "#" in child,
                            "Invalid type reference",
                        )
                        name = child.split("#", 1)[0]
                        safe_path(name)
                        require(name in names, "Type index references a missing file")
                    else:
                        check_refs(child)
            elif isinstance(value, list):
                for child in value:
                    check_refs(child)

        check_refs(index)


def validate_bundle(directory, expected):
    directory = Path(directory)
    files = set()
    for path in directory.rglob("*"):
        require(not path.is_symlink(), "Bundle links are forbidden")
        if path.is_file():
            require(not path.stat().st_mode & 0o111, "Executable bundle file")
            files.add(path.relative_to(directory).as_posix())
        else:
            require(path.is_dir(), "Unsupported bundle entry")
    require(
        len(files) <= MAX_FILES
        and sum((directory / name).stat().st_size for name in files) <= MAX_BYTES,
        "Bundle exceeds limits",
    )
    metadata = read_json(directory / "source.json")
    require(
        isinstance(metadata, dict)
        and set(metadata)
        == {"schemaVersion", "extension", "source", "bicepVersion", "manifestDigest"}
        and type(metadata["schemaVersion"]) is int
        and metadata["schemaVersion"] == 1
        and metadata["extension"] == "radius",
        "Invalid bundle metadata",
    )
    source = metadata["source"]
    require(
        isinstance(source, dict) and set(source) == set(expected),
        "Invalid source fields",
    )
    require(
        type(source["generationAttempt"]) is int
        and 1 <= source["generationAttempt"] <= expected["generationAttempt"],
        "Invalid generation attempt",
    )
    require(
        all(
            source[key] == expected[key]
            for key in expected
            if key != "generationAttempt"
        ),
        "Bundle source identity mismatch",
    )
    require(
        metadata["bicepVersion"] == pinned_version("BICEP"),
        "Bundle Bicep version mismatch",
    )
    layout = directory / "layout"
    require(
        read_json(layout / "oci-layout") == {"imageLayoutVersion": "1.0.0"},
        "Invalid OCI layout version",
    )
    index = read_json(layout / "index.json")
    require(
        isinstance(index, dict)
        and set(index) <= {"schemaVersion", "mediaType", "manifests"}
        and index["schemaVersion"] == 2
        and isinstance(index["manifests"], list)
        and len(index["manifests"]) == 1,
        "Expected one OCI root manifest",
    )
    root = index["manifests"][0]
    require(
        root["digest"] == metadata["manifestDigest"], "Bundle manifest digest mismatch"
    )
    raw = descriptor_blob(layout, root, MANIFEST_TYPE)
    manifest = decode_json(raw)
    require(
        isinstance(manifest, dict)
        and set(manifest)
        == {
            "schemaVersion",
            "mediaType",
            "artifactType",
            "config",
            "layers",
            "annotations",
        }
        and manifest["schemaVersion"] == 2
        and manifest["mediaType"] == MANIFEST_TYPE
        and manifest["artifactType"] == ARTIFACT_TYPE,
        "Invalid Bicep OCI manifest",
    )
    annotations = manifest["annotations"]
    require(
        isinstance(annotations, dict)
        and set(annotations)
        == {"bicep.serialization.format", "org.opencontainers.image.created"}
        and annotations["bicep.serialization.format"] == "v1"
        and isinstance(annotations["org.opencontainers.image.created"], str),
        "Invalid native Bicep annotations",
    )
    require(
        descriptor_blob(layout, manifest["config"], CONFIG_TYPE) == b"{}",
        "Executable or invalid provider config",
    )
    require(
        isinstance(manifest["layers"], list) and len(manifest["layers"]) == 1,
        "Executable or additional extension layers are forbidden",
    )
    layer = manifest["layers"][0]
    validate_types(descriptor_blob(layout, layer, LAYER_TYPE))
    allowed = {"source.json", "layout/index.json", "layout/oci-layout"}
    allowed.update(
        "layout/blobs/sha256/" + item["digest"].split(":")[1]
        for item in (root, manifest["config"], layer)
    )
    require(files == allowed, "Unexpected or unreferenced bundle files")
    return metadata, manifest


def pinned_version(prefix):
    text = (ROOT / "build/tools.generated.mk").read_text()
    match = re.search(rf"^{prefix}_VERSION \?= v([0-9.]+)$", text, re.MULTILINE)
    require(match is not None, f"Missing pinned {prefix} version")
    return match.group(1)


def verify_tool(prefix):
    if prefix == "BICEP":
        text = command("bicep", "--version").decode()
        pattern = r"^Bicep CLI version ([0-9.]+)(?:\s|$)"
    else:
        text = command("oras", "version").decode()
        pattern = r"^Version:\s+([0-9.]+)\s*$"
    match = re.search(pattern, text, re.MULTILINE)
    require(
        match is not None and match.group(1) == pinned_version(prefix),
        f"Pinned {prefix} is required",
    )


def command(*args, cwd=None, env=None, timeout=180):
    result = subprocess.run(
        args, cwd=cwd, env=env, capture_output=True, timeout=timeout, check=False
    )
    if result.returncode:
        raise RuntimeError(
            f"{args[0]} {args[1]} failed: {result.stderr.decode(errors='replace').strip()}"
        )
    return result.stdout


def capture(source_file, index_file, registry, destination):
    require(
        re.fullmatch(r"localhost:[0-9]+", registry),
        "Generation must use a loopback registry",
    )
    destination = Path(destination).resolve()
    require(not destination.exists(), "Bundle destination must be fresh")
    source = read_json(source_file)
    verify_tool("BICEP")
    verify_tool("ORAS")
    destination.mkdir(parents=True)
    with tempfile.TemporaryDirectory(prefix="bicep-package-") as scratch:
        scratch = Path(scratch)
        write_json(
            scratch / "bicepconfig.json",
            {"experimentalFeaturesEnabled": {"ociEnabled": True}},
        )
        write_json(scratch / "docker/config.json", {})
        env = {
            **os.environ,
            "DOCKER_CONFIG": str(scratch / "docker"),
            "BICEP_TRUSTED_REGISTRIES": "localhost",
        }
        command(
            "bicep",
            "publish-extension",
            str(Path(index_file).resolve()),
            "--target",
            f"br:{registry}/radius:bundle",
            cwd=scratch,
            env=env,
        )
        command(
            "oras",
            "cp",
            "--from-plain-http",
            "--to-oci-layout",
            f"{registry}/radius:bundle",
            f"{destination / 'layout'}:bundle",
            env=env,
        )
    manifest_digest = read_json(destination / "layout/index.json")["manifests"][0][
        "digest"
    ]
    write_json(
        destination / "source.json",
        {
            "schemaVersion": 1,
            "extension": "radius",
            "source": source,
            "bicepVersion": pinned_version("BICEP"),
            "manifestDigest": manifest_digest,
        },
    )
    validate_bundle(destination, source)


def stamp(directory, metadata, manifest, destination):
    destination = Path(destination)
    require(not destination.exists(), "Prepared layout must be fresh")
    blobs = destination / "blobs/sha256"
    blobs.mkdir(parents=True)
    for descriptor in [manifest["config"], *manifest["layers"]]:
        name = descriptor["digest"].split(":")[1]
        shutil.copyfile(Path(directory) / "layout/blobs/sha256" / name, blobs / name)
    stamped = {
        **manifest,
        "annotations": {
            **manifest["annotations"],
            "org.opencontainers.image.source": f"https://github.com/{REPOSITORY}",
            "org.opencontainers.image.revision": metadata["source"]["commit"],
        },
    }
    raw = (json.dumps(stamped, sort_keys=True, separators=(",", ":")) + "\n").encode()
    final_digest = digest(raw)
    (blobs / final_digest.split(":")[1]).write_bytes(raw)
    write_json(destination / "oci-layout", {"imageLayoutVersion": "1.0.0"})
    write_json(
        destination / "index.json",
        {
            "schemaVersion": 2,
            "manifests": [
                {
                    "mediaType": MANIFEST_TYPE,
                    "digest": final_digest,
                    "size": len(raw),
                    "annotations": {"org.opencontainers.image.ref.name": "bundle"},
                }
            ],
        },
    )
    return final_digest


def prepare(api, artifact_id, artifact_digest, directory):
    expected = source_context(api)
    artifact_id = positive(artifact_id)
    artifact = api.get(f"/repos/{REPOSITORY}/actions/artifacts/{artifact_id}")
    validate_artifact(artifact, expected, bundle_name(expected))
    if re.fullmatch(r"[0-9a-f]{64}", artifact_digest):
        artifact_digest = "sha256:" + artifact_digest
    require(artifact["digest"] == artifact_digest, "Selected artifact digest changed")
    directory = Path(directory)
    extract_artifact(api, artifact, directory / "bundle")
    metadata, manifest = validate_bundle(directory / "bundle", expected)
    final_digest = stamp(directory / "bundle", metadata, manifest, directory / "layout")
    previous = None
    if expected["generationAttempt"] > 1:
        entries = artifacts(api, expected)
        attempt = expected["generationAttempt"] - 1
        found = find_artifact(entries, expected, receipt_name(expected, attempt))
        if found:
            extract_artifact(api, found, directory / "previous")
            require(
                list((directory / "previous").iterdir())
                == [directory / "previous/receipt.json"],
                "Invalid receipt artifact",
            )
            previous = read_json(directory / "previous/receipt.json")
            require(
                previous["schemaVersion"] == 1
                and previous["extension"] == "radius"
                and previous["source"] == metadata["source"]
                and previous["manifestDigest"] == final_digest
                and previous["publicationAttempt"] == attempt
                and previous["phase"]
                in (
                    "prepared",
                    "writing-ghcr",
                    "writing-acr",
                    "verified",
                    "superseded",
                ),
                "Previous receipt identity or progress mismatch",
            )
    state = {"source": expected, "previous": previous}
    write_json(directory / "state.json", state)
    unresolved = previous and previous["phase"] in ("writing-ghcr", "writing-acr")
    write_json(
        directory / "receipt.json",
        {
            "schemaVersion": 1,
            "extension": "radius",
            "source": metadata["source"],
            "bundle": {
                "artifactId": artifact_id,
                "artifactDigest": artifact_digest,
                "manifestDigest": metadata["manifestDigest"],
            },
            "manifestDigest": final_digest,
            "publicationAttempt": expected["generationAttempt"],
            "destinations": {
                "ghcr": {"reference": GHCR, "digest": None},
                "acr": {"reference": ACR, "digest": None},
            },
            "status": "partial" if unresolved else "failed",
            "phase": previous["phase"] if unresolved else "prepared",
        },
    )


def verify_manifest(reference, expected, plain_http=False):
    flags = ["--plain-http"] if plain_http else []
    raw = command("oras", "manifest", "fetch", *flags, reference, timeout=60)
    require(digest(raw) == expected, f"Unexpected manifest digest at {reference}")
    return raw


def copy_exact(source, target, expected, from_layout=False, plain_http=False):
    flags = (
        ["--from-oci-layout"]
        if from_layout
        else (["--from-plain-http"] if plain_http else [])
    )
    if plain_http:
        flags.append("--to-plain-http")
    for attempt in range(3):
        try:
            command("oras", "cp", *flags, source, target)
            verify_manifest(target, expected, plain_http)
            return
        except (RuntimeError, subprocess.TimeoutExpired) as error:
            transient = isinstance(error, subprocess.TimeoutExpired) or re.search(
                r"\b(429|500|502|503|504)\b|timed? out|timeout",
                str(error),
                re.IGNORECASE,
            )
            if not transient:
                raise
            # Copying this exact content is idempotent; reconcile a lost response first.
            try:
                verify_manifest(target, expected, plain_http)
                return
            except (RuntimeError, ValueError, subprocess.TimeoutExpired):
                if attempt == 2:
                    raise error
                print(
                    f"::warning::Transient OCI copy failure; retrying the same digest ({attempt + 1}/2)"
                )
                time.sleep(2**attempt)
    raise RuntimeError("OCI copy retries exhausted")


def publish(api, directory):
    directory = Path(directory)
    expected = source_context(api)
    state, receipt = read_json(directory / "state.json"), read_json(
        directory / "receipt.json"
    )
    require(expected == state["source"], "Prepared publication context changed")
    require(
        receipt["destinations"]["ghcr"]["reference"] == GHCR
        and receipt["destinations"]["acr"]["reference"] == ACR,
        "Unexpected publishing destination",
    )
    verify_tool("ORAS")
    package = api.get("/orgs/radius-project/packages/container/bicep-types-radius")
    require(
        package["visibility"] == "public",
        "Preprovision the public production package before activation",
    )
    main = api.get(f"/repos/{REPOSITORY}/branches/main")
    require(
        main["protected"] is True
        and re.fullmatch(r"[0-9a-f]{40}", main["commit"]["sha"]),
        "Cannot verify protected main",
    )
    final_digest = receipt["manifestDigest"]
    require(valid_digest(final_digest), "Invalid final manifest digest")
    previous = state["previous"]
    if main["commit"]["sha"] != expected["commit"]:
        unresolved = previous and previous["phase"] in ("writing-ghcr", "writing-acr")
        if unresolved:
            receipt.update(status="partial", phase=previous["phase"])
            write_json(directory / "receipt.json", receipt)
            verify_manifest(GHCR, final_digest)
            print(
                "Reconciling a previously started pair without moving GHCR edge backwards"
            )
        else:
            require(
                expected["generationAttempt"] == 1 or previous is not None,
                "Superseded retry has no publication receipt; prior progress is unknown",
            )
            require(
                previous is None
                or previous["phase"] in ("prepared", "verified", "superseded"),
                "Previous publication progress is invalid",
            )
            receipt.update(status="superseded", phase="superseded")
            write_json(directory / "receipt.json", receipt)
            output(status="superseded", manifest_digest=final_digest)
            print("Source main snapshot is superseded; no registry mutation")
            return
    else:
        receipt.update(status="partial", phase="writing-ghcr")
        write_json(directory / "receipt.json", receipt)
        copy_exact(
            f"{directory / 'layout'}@{final_digest}",
            GHCR,
            final_digest,
            from_layout=True,
        )
    receipt["destinations"]["ghcr"]["digest"] = final_digest
    receipt.update(status="partial", phase="writing-acr")
    write_json(directory / "receipt.json", receipt)
    copy_exact(f"{GHCR.rsplit(':', 1)[0]}@{final_digest}", ACR, final_digest)
    require(
        verify_manifest(GHCR, final_digest) == verify_manifest(ACR, final_digest),
        "GHCR and ACR manifest bytes differ",
    )
    receipt["destinations"]["acr"]["digest"] = final_digest
    receipt.update(status="published", phase="verified")
    write_json(directory / "receipt.json", receipt)
    output(status="published", manifest_digest=final_digest)
    print(f"Published {GHCR} and {ACR} at {final_digest}")


def summary(mode, mode_result, legacy_result, direct_result, publication_status):
    require(mode_result == "success", "Bicep mode selection/change detection failed")
    if mode == "skip":
        require(
            legacy_result == direct_result == "skipped",
            "Unexpected publishing on an ineligible build",
        )
    elif mode == "legacy":
        require(
            legacy_result == "success" and direct_result == "skipped",
            "Legacy Bicep publishing failed or was skipped",
        )
    elif mode == "direct":
        require(
            legacy_result == "skipped"
            and direct_result == "success"
            and publication_status in ("published", "superseded"),
            "Direct Bicep publishing failed, was unexpectedly skipped, or returned no receipt status",
        )
    else:
        raise ValueError("Missing or invalid Bicep publishing mode")


def report(directory):
    receipt = read_json(Path(directory) / "receipt.json")
    lines = [
        "## Radius Bicep edge publication",
        "",
        f"Status: **{receipt['status']}** ({receipt['phase']})",
        f"Source: `{receipt['source']['repository']}@{receipt['source']['commit']}`",
        f"Bundle artifact: `{receipt['bundle']['artifactId']}`",
        f"Final manifest: `{receipt['manifestDigest']}`",
        "",
        "| Destination | Verified digest |",
        "| --- | --- |",
    ]
    for destination in receipt["destinations"].values():
        lines.append(
            f"| `{destination['reference']}` | `{destination['digest'] or 'not verified'}` |"
        )
    with open(os.environ["GITHUB_STEP_SUMMARY"], "a", encoding="utf-8") as stream:
        stream.write("\n".join(lines) + "\n")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="operation", required=True)
    select_args = commands.add_parser("select")
    select_args.add_argument("--directory", required=True)
    bundle = commands.add_parser("bundle")
    bundle.add_argument("--source", required=True)
    bundle.add_argument("--index", required=True)
    bundle.add_argument("--registry", required=True)
    bundle.add_argument("--directory", required=True)
    prepare_args = commands.add_parser("prepare")
    prepare_args.add_argument("--artifact-id", required=True)
    prepare_args.add_argument("--artifact-digest", required=True)
    prepare_args.add_argument("--directory", required=True)
    publish_args = commands.add_parser("publish")
    publish_args.add_argument("--directory", required=True)
    report_args = commands.add_parser("report")
    report_args.add_argument("--directory", required=True)
    commands.add_parser("summary")
    args = parser.parse_args()
    if args.operation == "bundle":
        capture(args.source, args.index, args.registry, args.directory)
    elif args.operation == "report":
        report(args.directory)
    elif args.operation == "summary":
        summary(
            *(
                os.environ.get(key, "")
                for key in (
                    "BICEP_MODE",
                    "BICEP_MODE_RESULT",
                    "BICEP_LEGACY_RESULT",
                    "BICEP_DIRECT_RESULT",
                    "BICEP_PUBLICATION_STATUS",
                )
            )
        )
    else:
        api = GitHub()
        if args.operation == "select":
            select(api, args.directory)
        elif args.operation == "prepare":
            prepare(api, args.artifact_id, args.artifact_digest, args.directory)
        else:
            publish(api, args.directory)


if __name__ == "__main__":
    try:
        main()
    except (
        ValueError,
        KeyError,
        OSError,
        RuntimeError,
        subprocess.SubprocessError,
        zipfile.BadZipFile,
        tarfile.TarError,
        URLError,
    ) as error:
        print(f"::error::{error}", file=sys.stderr)
        sys.exit(1)
