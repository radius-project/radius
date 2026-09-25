#!/usr/bin/env python3
"""Export and copy cloud-test OCI data without executing candidate artifacts."""

import argparse
import hashlib
import json
import os
import re
import shutil
import stat
import subprocess
import tempfile
import zipfile
from datetime import datetime
from pathlib import Path, PurePosixPath
from xml.etree import ElementTree

REPOSITORY = "radius-project/radius"
REGISTRY = "ghcr.io/radius-project/dev"
TYPES_REGISTRY = "crradfunctest1b2s.azurecr.io"
IMAGES = (
    "ucpd", "applications-rp", "dynamic-rp", "controller",
    "testrp", "magpiego", "bicep", "pre-upgrade",
)
OCI_MANIFEST = "application/vnd.oci.image.manifest.v1+json"
DOCKER_MANIFEST = "application/vnd.docker.distribution.manifest.v2+json"
NAME = re.compile(r"[a-z0-9][a-z0-9_.-]{0,95}")
DIGEST = re.compile(r"sha256:[0-9a-f]{64}")
MAX_ARCHIVE_BYTES = 4 * 1024**3
MAX_BUNDLE_BYTES = 12 * 1024**3
MAX_BLOB_BYTES = 2 * 1024**3
MAX_JSON_BYTES = 1024**2
MAX_FILES = 8192
MAX_RECIPES = 128
RESULT_NAMES = {
    "functional_test_results_corerp-cloud",
    "functional_test_results_ucp-cloud",
}


def require(condition, message):
    if not condition:
        raise ValueError(message)


def environment(name):
    value = os.environ.get(name, "")
    require(bool(value), f"{name} is required")
    return value


def number(value):
    require(re.fullmatch(r"[1-9][0-9]{0,19}", str(value)), "invalid numeric identity")
    return int(value)


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result, f"duplicate JSON key: {key}")
        result[key] = value
    return result


def parse_json(data):
    return json.loads(data, object_pairs_hook=unique_object)


def read_json(path):
    require(path.is_file() and not path.is_symlink(), f"missing regular file: {path}")
    require(path.stat().st_size <= MAX_JSON_BYTES, f"oversized JSON: {path}")
    return parse_json(path.read_bytes())


def digest_file(path):
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024**2), b""):
            digest.update(block)
    return f"sha256:{digest.hexdigest()}"


def command(*args):
    return subprocess.run(args, check=True, stdout=subprocess.PIPE).stdout


def api(path):
    return parse_json(command("gh", "api", path))


def source_identity(attempt):
    repository = environment("GITHUB_REPOSITORY")
    require(repository == REPOSITORY, "cloud uploads require the canonical repository")
    source_repository = environment("CHECKOUT_REPO")
    require(re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", source_repository),
            "invalid source repository")
    commit = environment("CHECKOUT_REF")
    workflow_sha = environment("GITHUB_SHA")
    require(re.fullmatch(r"[0-9a-f]{40}", commit), "source must be an immutable commit")
    require(re.fullmatch(r"[0-9a-f]{40}", workflow_sha), "invalid workflow commit")
    return {
        "repository": source_repository,
        "commit": commit,
        "workflowRepository": repository,
        "workflowSHA": workflow_sha,
        "runHeadSHA": run_head_sha(),
        "runId": str(number(environment("GITHUB_RUN_ID"))),
        "generationAttempt": number(attempt),
    }


def run_head_sha():
    # Actions records the PR candidate, not the controller SHA, for PR-target runs.
    name = "CHECKOUT_REF" if environment("GITHUB_EVENT_NAME") == "pull_request_target" else "GITHUB_SHA"
    value = environment(name)
    require(re.fullmatch(r"[0-9a-f]{40}", value), "invalid Actions run head SHA")
    return value


def tag():
    value = environment("REL_VERSION")
    require(re.fullmatch(r"pr-func[0-9a-f]{10}", value), "invalid cloud-test tag")
    return value


def artifact_path(kind, name):
    require(isinstance(name, str) and NAME.fullmatch(name), "invalid artifact basename")
    require(kind in ("images", "recipes", "types"), "invalid artifact kind")
    if kind == "images":
        require(name in IMAGES, "unexpected container image")
    if kind == "types":
        require(name == "radius", "unexpected Bicep extension")
    return f"{kind}/{name}"


def allowed_path(name, directory=False):
    path = PurePosixPath(name)
    require(not path.is_absolute() and str(path) == name and "\\" not in name,
            f"unsafe artifact path: {name}")
    parts = path.parts
    require(parts and all(part not in ("", ".", "..") for part in parts),
            f"unsafe artifact path: {name}")
    if name == "bundle.json":
        require(not directory, "bundle.json must be a file")
        return
    require(parts[0] in ("images", "recipes", "types"), f"unexpected path: {name}")
    if len(parts) == 1:
        require(directory, f"expected directory: {name}")
        return
    artifact_path(parts[0], parts[1])
    if len(parts) == 2:
        require(directory, f"expected layout directory: {name}")
        return
    if len(parts) == 3 and parts[2] in ("index.json", "oci-layout"):
        require(not directory, f"expected layout file: {name}")
        return
    require(parts[2] == "blobs", f"unexpected layout path: {name}")
    if len(parts) == 3:
        require(directory, "blobs must be a directory")
        return
    require(parts[3] == "sha256", "only SHA-256 blobs are supported")
    if len(parts) == 4:
        require(directory, "sha256 must be a directory")
        return
    require(len(parts) == 5 and not directory and
            re.fullmatch(r"[0-9a-f]{64}", parts[4]), "invalid blob path")


def annotations(value):
    require(isinstance(value, dict) and all(
        isinstance(key, str) and isinstance(item, str)
        for key, item in value.items()), "invalid OCI annotations")


def descriptor(root, value, media_types):
    require(isinstance(value, dict), "invalid OCI descriptor")
    require(set(value) <= {"mediaType", "digest", "size", "annotations", "artifactType"},
            "unsupported descriptor fields (including remote URLs)")
    require(value.get("mediaType") in media_types, "unexpected descriptor media type")
    digest = value.get("digest", "")
    require(isinstance(digest, str) and DIGEST.fullmatch(digest), "invalid blob digest")
    size = value.get("size")
    require(type(size) is int and 0 <= size <= MAX_BLOB_BYTES, "invalid blob size")
    if "annotations" in value:
        annotations(value["annotations"])
    path = root / "blobs" / "sha256" / digest.removeprefix("sha256:")
    require(path.is_file() and not path.is_symlink(), f"missing blob: {digest}")
    require(path.stat().st_size == size, f"blob size mismatch: {digest}")
    require(digest_file(path) == digest, f"blob digest mismatch: {digest}")
    return path


def validate_layout(root, kind, expected_digest):
    require(read_json(root / "oci-layout") == {"imageLayoutVersion": "1.0.0"},
            "unsupported OCI layout version")
    index = read_json(root / "index.json")
    require(isinstance(index, dict) and
            set(index) <= {"schemaVersion", "mediaType", "manifests", "annotations"},
            "invalid OCI index")
    require(index.get("schemaVersion") == 2 and
            index.get("mediaType", "application/vnd.oci.image.index.v1+json") ==
            "application/vnd.oci.image.index.v1+json", "invalid OCI index type")
    manifests = index.get("manifests")
    require(isinstance(manifests, list) and len(manifests) == 1,
            "exactly one OCI manifest is required")
    root_descriptor = manifests[0]
    manifest_types = {OCI_MANIFEST, DOCKER_MANIFEST} if kind == "images" else {OCI_MANIFEST}
    manifest_path = descriptor(root, root_descriptor, manifest_types)
    require(root_descriptor["digest"] == expected_digest, "manifest digest mismatch")
    require(root_descriptor.get("annotations", {}).get(
        "org.opencontainers.image.ref.name") == "payload", "unexpected layout reference")
    manifest = read_json(manifest_path)
    require(isinstance(manifest, dict) and set(manifest) <= {
        "schemaVersion", "mediaType", "artifactType", "config", "layers", "annotations",
    }, "unsupported manifest fields (including subjects)")
    require(manifest.get("schemaVersion") == 2 and
            manifest.get("mediaType", root_descriptor["mediaType"]) ==
            root_descriptor["mediaType"], "invalid manifest type")
    if "annotations" in manifest:
        annotations(manifest["annotations"])
    layers = manifest.get("layers")
    require(isinstance(layers, list) and 1 <= len(layers) <= 128, "invalid layer count")
    if kind == "images":
        require("artifactType" not in manifest, "unexpected image artifact type")
        config_types = {
            "application/vnd.oci.image.config.v1+json",
            "application/vnd.docker.container.image.v1+json",
        }
        layer_types = {
            "application/vnd.oci.image.layer.v1.tar",
            "application/vnd.oci.image.layer.v1.tar+gzip",
            "application/vnd.oci.image.layer.v1.tar+zstd",
            "application/vnd.docker.image.rootfs.diff.tar.gzip",
        }
    else:
        family = "provider" if kind == "types" else "module"
        artifact_type = f"application/vnd.ms.bicep.{family}.artifact"
        # rad bicep publish emits an OCI 1.0 module without artifactType.
        require(manifest.get("artifactType") in
                ({artifact_type} if kind == "types" else {None, artifact_type}),
                "unexpected Bicep artifact type")
        require(len(layers) == 1, "exactly one Bicep layer is required")
        config_types = {f"application/vnd.ms.bicep.{family}.config.v1+json"}
        suffix = ".tar+gzip" if kind == "types" else "+json"
        layer_types = {f"application/vnd.ms.bicep.{family}.layer.v1{suffix}"}
    config_path = descriptor(root, manifest.get("config"), config_types)
    if kind != "images":
        require(read_json(config_path) == {}, "Bicep config must be an empty object")
    expected_files = {"index.json", "oci-layout", str(manifest_path.relative_to(root)),
                      str(config_path.relative_to(root))}
    for layer in layers:
        path = descriptor(root, layer, layer_types)
        expected_files.add(str(path.relative_to(root)))
    actual_files = {str(path.relative_to(root)) for path in root.rglob("*") if path.is_file()}
    require(actual_files == expected_files, "unexpected or unreferenced layout files")


def validate_bundle(root, attempt):
    require(root.is_dir() and not root.is_symlink(), "missing artifact directory")
    total = 0
    paths = list(root.rglob("*"))
    require(len(paths) <= MAX_FILES, "too many artifact entries")
    for path in paths:
        mode = path.lstat().st_mode
        require(stat.S_ISREG(mode) or stat.S_ISDIR(mode), "non-regular artifact entry")
        allowed_path(path.relative_to(root).as_posix(), path.is_dir())
        if path.is_file():
            total += path.stat().st_size
    require(total <= MAX_BUNDLE_BYTES, "artifact exceeds expanded size limit")
    bundle = read_json(root / "bundle.json")
    require(isinstance(bundle, dict) and
            set(bundle) == {"schemaVersion", "source", "tag", "artifacts"},
            "invalid cloud-test bundle")
    require(bundle["schemaVersion"] == 1, "unsupported bundle schema")
    require(bundle["source"] == source_identity(attempt), "source/run identity mismatch")
    require(bundle["tag"] == tag(), "test tag mismatch")
    artifacts = bundle["artifacts"]
    require(isinstance(artifacts, list) and
            len(IMAGES) + 2 <= len(artifacts) <= len(IMAGES) + 1 + MAX_RECIPES,
            "incomplete or oversized artifact set")
    seen = set()
    for artifact in artifacts:
        require(isinstance(artifact, dict) and
                set(artifact) == {"kind", "name", "path", "digest"}, "invalid artifact record")
        path = artifact_path(artifact["kind"], artifact["name"])
        require(artifact["path"] == path and path not in seen, "duplicate or unsafe layout path")
        seen.add(path)
        validate_layout(root / path, artifact["kind"], artifact["digest"])
    required = {f"images/{name}" for name in IMAGES} | {"types/radius"}
    require(required <= seen, "missing required image or Radius types")
    actual = {str(path.relative_to(root)) for kind in ("images", "recipes", "types")
              for path in (root / kind).iterdir()}
    require(actual == seen, "unlisted artifact layout")
    return bundle


def export_bundle(root):
    root.mkdir(parents=True, exist_ok=False)
    recipes = sorted(path.stem for path in
                     Path("test/testrecipes/test-bicep-recipes").rglob("*.bicep")
                     if not path.name.startswith("_"))
    require(0 < len(recipes) <= MAX_RECIPES and len(set(recipes)) == len(recipes),
            "missing, duplicate, or excessive recipe names")
    records = []
    inputs = [("images", name, f"images/{name}") for name in IMAGES]
    inputs += [("types", "radius", "test/radius")]
    inputs += [("recipes", name, f"test/testrecipes/test-bicep-recipes/{name}")
               for name in recipes]
    for kind, name, local_path in inputs:
        path = artifact_path(kind, name)
        layout = root / path
        layout.parent.mkdir(parents=True, exist_ok=True)
        subprocess.run(["oras", "cp", "--to-oci-layout",
                        f"localhost:5000/{local_path}:{tag()}",
                        f"{layout}:payload"], check=True)
        index = read_json(layout / "index.json")
        records.append({"kind": kind, "name": name, "path": path,
                        "digest": index["manifests"][0]["digest"]})
    attempt = environment("GITHUB_RUN_ATTEMPT")
    bundle = {"schemaVersion": 1, "source": source_identity(attempt),
              "tag": tag(), "artifacts": records}
    (root / "bundle.json").write_text(json.dumps(bundle) + "\n")
    validate_bundle(root, attempt)


def timestamp(value):
    return datetime.fromisoformat(value.replace("Z", "+00:00"))


def verify_metadata(metadata, result_name=None):
    run_id = number(environment("GITHUB_RUN_ID"))
    run = metadata.get("workflow_run", {})
    require(run.get("id") == run_id and
            run.get("repository_id") == number(environment("GITHUB_REPOSITORY_ID")) and
            run.get("head_sha") == run_head_sha(),
            "artifact is not from this workflow run/repository/commit")
    require(metadata.get("expired") is False, "artifact has expired")
    size = metadata.get("size_in_bytes")
    limit = 20 * 1024**2 if result_name else MAX_ARCHIVE_BYTES
    require(type(size) is int and 0 < size <= limit, "invalid artifact archive size")
    require(DIGEST.fullmatch(metadata.get("digest", "")), "missing archive digest")
    number(metadata.get("id", ""))
    if result_name:
        require(result_name in RESULT_NAMES and metadata.get("name") == result_name,
                "unexpected test-results artifact")
        return None
    match = re.fullmatch(rf"cloud-test-inputs-{run_id}-([1-9][0-9]*)",
                         metadata.get("name", ""))
    require(match is not None, "unexpected cloud-test artifact name")
    attempt = number(match.group(1))
    current_attempt = number(environment("GITHUB_RUN_ATTEMPT"))
    require(attempt <= current_attempt, "artifact came from a future attempt")
    attempt_run = api(f"repos/{REPOSITORY}/actions/runs/{run_id}/attempts/{attempt}")
    require(attempt_run.get("id") == run_id and
            attempt_run.get("run_attempt") == attempt and
            attempt_run.get("head_sha") == run_head_sha() and
            attempt_run.get("event") == environment("GITHUB_EVENT_NAME") and
            attempt_run.get("repository", {}).get("full_name") == REPOSITORY and
            attempt_run.get("path", "").split("@")[0] ==
            ".github/workflows/functional-test-cloud.yaml", "producing attempt mismatch")
    created = timestamp(metadata["created_at"])
    require(created >= timestamp(attempt_run["run_started_at"]), "artifact predates producer")
    if attempt < current_attempt:
        next_run = api(f"repos/{REPOSITORY}/actions/runs/{run_id}/attempts/{attempt + 1}")
        require(created <= timestamp(next_run["run_started_at"]), "artifact postdates producer")
    return attempt


def extract_archive(archive, root, results=False):
    require(not root.exists(), "download destination must be fresh")
    with zipfile.ZipFile(archive) as source:
        entries = source.infolist()
        require(0 < len(entries) <= MAX_FILES, "invalid archive entry count")
        seen = set()
        total = 0
        for entry in entries:
            name = entry.filename.removesuffix("/")
            require(entry.orig_filename == entry.filename and name not in seen,
                    "duplicate or NUL-containing archive path")
            seen.add(name)
            mode = stat.S_IFMT(entry.external_attr >> 16)
            require(mode in (0, stat.S_IFREG, stat.S_IFDIR), "archive contains a link/device")
            require(not entry.flag_bits & 1, "encrypted archives are not supported")
            if results:
                require((entry.is_dir() and name == "processed") or
                        (not entry.is_dir() and name in ("results.xml", "processed/results.xml")),
                        "unexpected test-results path")
            else:
                allowed_path(name, entry.is_dir())
            total += entry.file_size
        require(total <= (20 * 1024**2 if results else MAX_BUNDLE_BYTES),
                "archive exceeds expanded size limit")
        root.mkdir(parents=True)
        for entry in entries:
            target = root / entry.filename
            if entry.is_dir():
                target.mkdir(parents=True, exist_ok=True)
                continue
            target.parent.mkdir(parents=True, exist_ok=True)
            with source.open(entry) as input_stream, target.open("xb") as output:
                shutil.copyfileobj(input_stream, output, 1024**2)
            require(target.stat().st_size == entry.file_size, "archive file size mismatch")


def download(root, results=False):
    source_identity(environment("GITHUB_RUN_ATTEMPT"))
    result_name = environment("RESULT_ARTIFACT_NAME") if results else None
    if results:
        require(result_name in RESULT_NAMES, "invalid results artifact name")
        response = api(f"repos/{REPOSITORY}/actions/runs/{number(environment('GITHUB_RUN_ID'))}"
                       f"/artifacts?name={result_name}&per_page=100")
        artifacts = response.get("artifacts", [])
        require(len(artifacts) == 1, "missing or ambiguous test-results artifact")
        artifact_id = number(artifacts[0]["id"])
    else:
        artifact_id = number(environment("ARTIFACT_ID"))
    metadata = api(f"repos/{REPOSITORY}/actions/artifacts/{artifact_id}")
    require(metadata.get("id") == artifact_id, "artifact ID mismatch")
    attempt = verify_metadata(metadata, result_name)
    root.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="cloud-artifact-", dir=root.parent) as temporary:
        archive = Path(temporary) / "artifact.zip"
        with archive.open("xb") as output:
            subprocess.run(["gh", "api",
                            f"repos/{REPOSITORY}/actions/artifacts/{artifact_id}/zip"],
                           stdout=output, check=True)
        limit = 20 * 1024**2 if results else MAX_ARCHIVE_BYTES
        require(archive.stat().st_size <= limit, "download exceeds archive size limit")
        require(digest_file(archive) == metadata["digest"], "archive digest mismatch")
        extract_archive(archive, root, results)
    if results:
        require((root / "processed/results.xml").is_file(), "missing processed test results")
        for path in root.rglob("*.xml"):
            content = path.read_text(encoding="utf-8")
            require("\0" not in content, "test result XML must be UTF-8")
            require("<!DOCTYPE" not in content.upper() and "<!ENTITY" not in content.upper(),
                    "external/entity XML declarations are forbidden")
            document = ElementTree.fromstring(content)
            require(document.tag in ("testsuites", "testsuite"), "expected JUnit XML")
    else:
        bundle = validate_bundle(root, attempt)
        root.with_suffix(".receipt.json").write_text(json.dumps({
            "artifactId": artifact_id, "archiveDigest": metadata["digest"],
            "source": bundle["source"], "tag": bundle["tag"],
        }) + "\n")


def upload(root, destination):
    require(environment("CONTAINER_REGISTRY") == REGISTRY and
            environment("BICEP_RECIPE_REGISTRY") == REGISTRY and
            environment("TEST_BICEP_TYPES_REGISTRY") == TYPES_REGISTRY,
            "configured test destinations are not allowlisted")
    receipt = read_json(root.with_suffix(".receipt.json"))
    require(receipt["artifactId"] == number(environment("ARTIFACT_ID")), "receipt ID mismatch")
    bundle = validate_bundle(root, receipt["source"]["generationAttempt"])
    require(receipt["source"] == bundle["source"] and receipt["tag"] == tag(),
            "receipt source mismatch")
    uploads = []
    for artifact in bundle["artifacts"]:
        kind, name = artifact["kind"], artifact["name"]
        if destination == "ghcr" and kind in ("images", "recipes"):
            path = name if kind == "images" else f"test/testrecipes/test-bicep-recipes/{name}"
            target = f"{REGISTRY}/{path}:{tag()}"
        elif destination == "acr" and kind == "types":
            target = f"{TYPES_REGISTRY}/test/radius:{tag()}"
        else:
            continue
        source = f"{root / artifact['path']}@{artifact['digest']}"
        subprocess.run(["oras", "cp", "--from-oci-layout", source, target], check=True)
        published = parse_json(command("oras", "manifest", "fetch", "--descriptor", target))
        require(published.get("digest") == artifact["digest"], "published digest changed")
        uploads.append({"target": target, "digest": artifact["digest"]})
    require(uploads, "no artifacts selected for upload")
    receipt["uploads"] = uploads
    root.with_suffix(f".{destination}-receipt.json").write_text(json.dumps(receipt) + "\n")
    with Path(environment("GITHUB_STEP_SUMMARY")).open("a") as summary:
        summary.write(f"## Cloud-test {destination} uploads\n\n")
        for item in uploads:
            summary.write(f"- `{item['target']}` @ `{item['digest']}`\n")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("operation", choices=("export", "download", "upload", "results"))
    parser.add_argument("directory", type=Path)
    parser.add_argument("--destination", choices=("ghcr", "acr"))
    args = parser.parse_args()
    if args.operation == "export":
        export_bundle(args.directory)
    elif args.operation in ("download", "results"):
        download(args.directory, args.operation == "results")
    else:
        require(args.destination is not None, "upload destination is required")
        upload(args.directory, args.destination)


if __name__ == "__main__":
    try:
        main()
    except (ValueError, KeyError, OSError, zipfile.BadZipFile,
            subprocess.CalledProcessError, ElementTree.ParseError) as error:
        raise SystemExit(f"::error::Cloud-test artifact operation failed: {error}") from error
