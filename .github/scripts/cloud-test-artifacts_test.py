#!/usr/bin/env python3
"""Offline contract tests for cloud-test artifacts and data-only uploads."""

import copy
import hashlib
import importlib.util
import json
import os
import stat
import subprocess
import tempfile
import unittest
import warnings
import zipfile
from pathlib import Path
from unittest.mock import patch

SCRIPT = Path(__file__).with_name("cloud-test-artifacts.py")
SPEC = importlib.util.spec_from_file_location("cloud_artifacts", SCRIPT)
artifacts = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(artifacts)


def write_json(path, value):
    path.write_text(json.dumps(value))


def blob(root, content, media_type):
    raw = content if isinstance(content, bytes) else json.dumps(content).encode()
    digest = hashlib.sha256(raw).hexdigest()
    directory = root / "blobs/sha256"
    directory.mkdir(parents=True, exist_ok=True)
    (directory / digest).write_bytes(raw)
    return {"mediaType": media_type, "digest": f"sha256:{digest}", "size": len(raw)}


def layout(root, kind):
    root.mkdir(parents=True)
    if kind == "images":
        config_type = "application/vnd.docker.container.image.v1+json"
        layer_type = "application/vnd.docker.image.rootfs.diff.tar.gzip"
        media_type = artifacts.DOCKER_MANIFEST
    elif kind == "types":
        config_type = "application/vnd.ms.bicep.provider.config.v1+json"
        layer_type = "application/vnd.ms.bicep.provider.layer.v1.tar+gzip"
        media_type = artifacts.OCI_MANIFEST
    else:
        config_type = "application/vnd.ms.bicep.module.config.v1+json"
        layer_type = "application/vnd.ms.bicep.module.layer.v1+json"
        media_type = artifacts.OCI_MANIFEST
    manifest = {
        "schemaVersion": 2,
        "config": blob(root, {}, config_type),
        "layers": [blob(root, b"opaque-payload-never-executed", layer_type)],
    }
    if kind != "recipes":
        manifest["mediaType"] = media_type
    if kind == "types":
        manifest["artifactType"] = "application/vnd.ms.bicep.provider.artifact"
    descriptor = blob(root, manifest, media_type)
    descriptor["annotations"] = {"org.opencontainers.image.ref.name": "payload"}
    write_json(root / "index.json", {"schemaVersion": 2, "manifests": [descriptor]})
    write_json(root / "oci-layout", {"imageLayoutVersion": "1.0.0"})
    return descriptor["digest"]


class CloudArtifactTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="cloud-artifacts-test-")
        self.addCleanup(self.temporary.cleanup)
        self.directory = Path(self.temporary.name)
        self.root = self.directory / "inputs"
        self.root.mkdir()
        self.env = {
            "GITHUB_REPOSITORY": artifacts.REPOSITORY,
            "GITHUB_REPOSITORY_ID": "340522752",
            "GITHUB_RUN_ID": "101",
            "GITHUB_RUN_ATTEMPT": "1",
            "GITHUB_SHA": "a" * 40,
            "GITHUB_EVENT_NAME": "pull_request_target",
            "CHECKOUT_REPO": "contributor/radius",
            "CHECKOUT_REF": "b" * 40,
            "REL_VERSION": "pr-func0123456789",
            "ARTIFACT_ID": "202",
            "CONTAINER_REGISTRY": artifacts.REGISTRY,
            "BICEP_RECIPE_REGISTRY": artifacts.REGISTRY,
            "TEST_BICEP_TYPES_REGISTRY": artifacts.TYPES_REGISTRY,
            "GITHUB_STEP_SUMMARY": str(self.directory / "summary.md"),
        }
        self.environment = patch.dict(os.environ, self.env)
        self.environment.start()
        self.addCleanup(self.environment.stop)
        records = []
        for kind, name in [
            *(("images", name) for name in artifacts.IMAGES),
            ("types", "radius"),
            ("recipes", "dynamicrp_recipe"),
            ("recipes", "redis-recipe"),
        ]:
            path = f"{kind}/{name}"
            records.append({"kind": kind, "name": name, "path": path,
                            "digest": layout(self.root / path, kind)})
        self.bundle = {"schemaVersion": 1, "source": artifacts.source_identity("1"),
                       "tag": self.env["REL_VERSION"], "artifacts": records}
        self.save_bundle()
        self.receipt = {"artifactId": 202, "archiveDigest": "sha256:" + "c" * 64,
                        "source": self.bundle["source"], "tag": self.bundle["tag"]}
        write_json(self.root.with_suffix(".receipt.json"), self.receipt)
        self.metadata = {
            "id": 202, "name": "cloud-test-inputs-101-1",
            "expired": False, "size_in_bytes": 5000,
            "digest": "sha256:" + "c" * 64, "created_at": "2026-09-25T10:10:00Z",
            "workflow_run": {"id": 101, "repository_id": 340522752, "head_sha": "a" * 40},
        }
        self.run = {
            "id": 101, "run_attempt": 1, "head_sha": "a" * 40,
            "event": "pull_request_target",
            "repository": {"full_name": artifacts.REPOSITORY},
            "path": ".github/workflows/functional-test-cloud.yaml",
            "run_started_at": "2026-09-25T10:00:00Z",
        }

    def save_bundle(self):
        write_json(self.root / "bundle.json", self.bundle)

    def replace_manifest(self, modify):
        record = self.bundle["artifacts"][0]
        root = self.root / record["path"]
        index = artifacts.read_json(root / "index.json")
        old = root / "blobs/sha256" / record["digest"][7:]
        manifest = artifacts.read_json(old)
        modify(manifest)
        descriptor = blob(root, manifest, artifacts.DOCKER_MANIFEST)
        descriptor["annotations"] = {"org.opencontainers.image.ref.name": "payload"}
        index["manifests"] = [descriptor]
        old.unlink()
        write_json(root / "index.json", index)
        record["digest"] = descriptor["digest"]
        self.save_bundle()

    def test_complete_payload_and_all_destinations_preserve_digests(self):
        self.assertEqual(artifacts.validate_bundle(self.root, "1"), self.bundle)
        calls = []
        target_digests = {}

        def copied(command, **kwargs):
            self.assertEqual(command[:3], ["oras", "cp", "--from-oci-layout"])
            self.assertNotIn("run", command)
            self.assertNotIn("build", command)
            target_digests[command[-1]] = command[-2].split("@")[1]
            calls.append(command)
            return subprocess.CompletedProcess(command, 0)

        def fetched(*command):
            self.assertEqual(command[:4], ("oras", "manifest", "fetch", "--descriptor"))
            return json.dumps({"digest": target_digests[command[-1]]}).encode()

        with patch.object(artifacts.subprocess, "run", side_effect=copied), \
                patch.object(artifacts, "command", side_effect=fetched):
            artifacts.upload(self.root, "ghcr")
            artifacts.upload(self.root, "acr")
        self.assertEqual(len(calls), len(self.bundle["artifacts"]))
        for name in artifacts.IMAGES:
            self.assertIn(f"{artifacts.REGISTRY}/{name}:{self.env['REL_VERSION']}", target_digests)
        self.assertIn(f"{artifacts.REGISTRY}/test/testrecipes/test-bicep-recipes/"
                      f"dynamicrp_recipe:{self.env['REL_VERSION']}", target_digests)
        self.assertIn(f"{artifacts.TYPES_REGISTRY}/test/radius:{self.env['REL_VERSION']}",
                      target_digests)
        self.assertTrue(self.root.with_suffix(".ghcr-receipt.json").is_file())
        self.assertTrue(self.root.with_suffix(".acr-receipt.json").is_file())

    def test_reject_metadata_source_and_target_tampering_before_any_upload(self):
        mutations = {
            "target field": lambda value: value.update(target="ghcr.io/radius-project/radius"),
            "tag": lambda value: value.update(tag="latest"),
            "source repo": lambda value: value["source"].update(repository="attacker/radius"),
            "source sha": lambda value: value["source"].update(commit="c" * 40),
            "source run": lambda value: value["source"].update(runId="99"),
            "source attempt": lambda value: value["source"].update(generationAttempt=2),
            "path": lambda value: value["artifacts"][0].update(path="../../uploader.py"),
            "host": lambda value: value["artifacts"][0].update(name="ghcr.io/radius"),
            "unexpected image": lambda value: value["artifacts"][0].update(name="radius"),
            "duplicates": lambda value: value["artifacts"].append(value["artifacts"][0]),
            "missing inputs": lambda value: value.update(artifacts=[]),
            "missing extension": lambda value: value.update(
                artifacts=[item for item in value["artifacts"] if item["kind"] != "types"]),
        }
        original = copy.deepcopy(self.bundle)
        for name, modify in mutations.items():
            with self.subTest(name=name):
                self.bundle = copy.deepcopy(original)
                modify(self.bundle)
                self.save_bundle()
                with patch.object(artifacts.subprocess, "run") as upload:
                    with self.assertRaises(ValueError):
                        artifacts.upload(self.root, "ghcr")
                    upload.assert_not_called()
        self.bundle = original
        self.save_bundle()
        for name in ("CONTAINER_REGISTRY", "BICEP_RECIPE_REGISTRY", "TEST_BICEP_TYPES_REGISTRY"):
            with self.subTest(destination=name), \
                    patch.dict(os.environ, {name: "biceptypes.azurecr.io"}), \
                    patch.object(artifacts.subprocess, "run") as upload:
                with self.assertRaises(ValueError):
                    artifacts.upload(self.root, "ghcr")
                upload.assert_not_called()

    def test_foreign_layer_urls_rejected_even_when_digests_match(self):
        self.replace_manifest(lambda manifest: manifest["layers"][0].update(
            urls=["https://example.invalid/credential-target"]))
        with self.assertRaisesRegex(ValueError, "descriptor fields"):
            artifacts.validate_bundle(self.root, "1")

    def test_unsupported_image_media_type_rejected(self):
        self.replace_manifest(lambda manifest: manifest["layers"][0].update(
            mediaType="application/vnd.docker.image.rootfs.foreign.diff.tar.gzip"))
        with self.assertRaisesRegex(ValueError, "media type"):
            artifacts.validate_bundle(self.root, "1")

    def test_subject_reference_rejected(self):
        self.replace_manifest(lambda manifest: manifest.update(subject=manifest["config"]))
        with self.assertRaisesRegex(ValueError, "manifest fields"):
            artifacts.validate_bundle(self.root, "1")

    def test_blob_digest_size_missing_and_symlink_rejected(self):
        record = self.bundle["artifacts"][0]
        root = self.root / record["path"]
        index = artifacts.read_json(root / "index.json")
        path = root / "blobs/sha256" / record["digest"][7:]
        original = path.read_bytes()
        for payload in (b"x" * len(original), original + b"x"):
            path.write_bytes(payload)
            with self.assertRaises(ValueError):
                artifacts.validate_bundle(self.root, "1")
        path.unlink()
        with self.assertRaises(ValueError):
            artifacts.validate_bundle(self.root, "1")
        outside = self.directory / "outside"
        outside.write_bytes(original)
        path.symlink_to(outside)
        with self.assertRaisesRegex(ValueError, "non-regular"):
            artifacts.validate_bundle(self.root, "1")
        path.unlink()
        path.write_bytes(original)
        index["manifests"].append(index["manifests"][0])
        write_json(root / "index.json", index)
        with self.assertRaisesRegex(ValueError, "exactly one"):
            artifacts.validate_bundle(self.root, "1")

    def test_unreferenced_files_and_real_size_count_limits(self):
        with patch.object(artifacts, "MAX_FILES", 1):
            with self.assertRaisesRegex(ValueError, "too many"):
                artifacts.validate_bundle(self.root, "1")
        with patch.object(artifacts, "MAX_BUNDLE_BYTES", 1):
            with self.assertRaisesRegex(ValueError, "size limit"):
                artifacts.validate_bundle(self.root, "1")
        with patch.object(artifacts, "MAX_BLOB_BYTES", 1):
            with self.assertRaisesRegex(ValueError, "blob size"):
                artifacts.validate_bundle(self.root, "1")
        extra = self.root / "images/ucpd/blobs/sha256" / ("d" * 64)
        extra.write_bytes(b"extra")
        with self.assertRaisesRegex(ValueError, "unreferenced"):
            artifacts.validate_bundle(self.root, "1")

    def test_artifact_metadata_and_downstream_only_reruns(self):
        with patch.object(artifacts, "api", return_value=self.run):
            self.assertEqual(artifacts.verify_metadata(self.metadata), 1)
        for field, value in [
            ("name", "cloud-test-inputs-100-1"), ("name", "cloud-test-inputs-101-2"),
            ("expired", True), ("size_in_bytes", artifacts.MAX_ARCHIVE_BYTES + 1),
            ("digest", ""), ("created_at", "2026-09-25T09:00:00Z"),
            ("workflow_run", {"id": 99, "repository_id": 340522752, "head_sha": "a" * 40}),
            ("workflow_run", {"id": 101, "repository_id": 99, "head_sha": "a" * 40}),
            ("workflow_run", {"id": 101, "repository_id": 340522752, "head_sha": "c" * 40}),
        ]:
            with self.subTest(field=field, value=value), \
                    patch.object(artifacts, "api", return_value=self.run):
                bad = copy.deepcopy(self.metadata)
                bad[field] = value
                with self.assertRaises(ValueError):
                    artifacts.verify_metadata(bad)
        next_run = {**self.run, "run_attempt": 2, "run_started_at": "2026-09-25T11:00:00Z"}
        with patch.dict(os.environ, {"GITHUB_RUN_ATTEMPT": "2"}), \
                patch.object(artifacts, "api", side_effect=[self.run, next_run]):
            self.assertEqual(artifacts.verify_metadata(self.metadata), 1)
        for field, value in [("path", ".github/workflows/other.yaml"),
                             ("event", "push"), ("head_sha", "c" * 40)]:
            with self.subTest(run_field=field), \
                    patch.object(artifacts, "api", return_value={**self.run, field: value}):
                with self.assertRaises(ValueError):
                    artifacts.verify_metadata(self.metadata)

    def test_archive_paths_and_links_rejected_before_extraction(self):
        for name in ("../escape", "/absolute", "images/../escape", "images//ucpd/index.json",
                     "images/ucpd/../../uploader", "recipes/_module/index.json",
                     "images/ucpd\\evil", "images/ucpd/blobs/sha256/not-a-digest"):
            with self.subTest(name=name):
                archive = self.directory / "bad.zip"
                with zipfile.ZipFile(archive, "w") as output:
                    output.writestr(name, b"bad")
                destination = self.directory / "extracted"
                with self.assertRaises(ValueError):
                    artifacts.extract_archive(archive, destination)
                self.assertFalse(destination.exists())
        info = zipfile.ZipInfo("images/ucpd/index.json")
        info.create_system = 3
        info.external_attr = (stat.S_IFLNK | 0o777) << 16
        with zipfile.ZipFile(archive, "w") as output:
            output.writestr(info, b"/etc/passwd")
        with self.assertRaisesRegex(ValueError, "link/device"):
            artifacts.extract_archive(archive, destination)
        with warnings.catch_warnings():
            warnings.simplefilter("ignore", UserWarning)
            with zipfile.ZipFile(archive, "w") as output:
                output.writestr("bundle.json", b"{}")
                output.writestr("bundle.json", b"{}")
        with self.assertRaisesRegex(ValueError, "duplicate"):
            artifacts.extract_archive(archive, destination)

    def test_complete_download_checks_archive_digest_and_immutable_id(self):
        archive = self.directory / "fixture.zip"
        with zipfile.ZipFile(archive, "w") as output:
            for path in self.root.rglob("*"):
                if path.is_file():
                    output.write(path, path.relative_to(self.root))
        self.metadata["digest"] = artifacts.digest_file(archive)
        self.metadata["size_in_bytes"] = archive.stat().st_size

        def download(command, stdout, **kwargs):
            self.assertEqual(command, ["gh", "api",
                                      "repos/radius-project/radius/actions/artifacts/202/zip"])
            stdout.write(archive.read_bytes())
            return subprocess.CompletedProcess(command, 0)

        target = self.directory / "downloaded"
        with patch.object(artifacts, "api", side_effect=[self.metadata, self.run]), \
                patch.object(artifacts.subprocess, "run", side_effect=download):
            artifacts.download(target)
        self.assertEqual(artifacts.validate_bundle(target, "1"), self.bundle)
        with patch.object(artifacts, "api", side_effect=[
                {**self.metadata, "digest": "sha256:" + "0" * 64}, self.run]), \
                patch.object(artifacts.subprocess, "run", side_effect=download):
            with self.assertRaisesRegex(ValueError, "archive digest"):
                artifacts.download(self.directory / "bad-digest")
        with patch.object(artifacts, "api", return_value={**self.metadata, "id": 203}):
            with self.assertRaisesRegex(ValueError, "ID mismatch"):
                artifacts.download(self.directory / "bad-id")

    def test_export_uses_only_local_data_and_preserves_recipe_names(self):
        calls = []

        def copy_layout(command, **kwargs):
            self.assertEqual(command[:3], ["oras", "cp", "--to-oci-layout"])
            self.assertTrue(command[-2].startswith("localhost:5000/"))
            output = Path(command[-1].removesuffix(":payload"))
            layout(output, output.parent.name)
            calls.append(command)
            return subprocess.CompletedProcess(command, 0)

        root = self.directory / "exported"
        with patch.object(artifacts.subprocess, "run", side_effect=copy_layout):
            artifacts.export_bundle(root)
        bundle = artifacts.validate_bundle(root, "1")
        recipes = [item["name"] for item in bundle["artifacts"] if item["kind"] == "recipes"]
        self.assertIn("dynamicrp_sensitive_recipe", recipes)
        self.assertNotIn("_resource-creation", recipes)
        self.assertEqual(len(calls), len(bundle["artifacts"]))
        with patch.object(artifacts.subprocess, "run", side_effect=subprocess.CalledProcessError(
                1, ["oras", "cp"])):
            failed = self.directory / "failed-export"
            with self.assertRaises(subprocess.CalledProcessError):
                artifacts.export_bundle(failed)
            self.assertFalse((failed / "bundle.json").exists())

    def test_cli_surfaces_invalid_input_without_upload(self):
        result = subprocess.run(
            ["python3", str(SCRIPT), "download", str(self.directory / "invalid")],
            env={**os.environ, "ARTIFACT_ID": "../../invalid"},
            text=True, capture_output=True, check=False,
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("::error::Cloud-test artifact operation failed", result.stderr)

    def test_duplicate_json_keys_are_rejected(self):
        (self.root / "bundle.json").write_text('{"schemaVersion":1,"schemaVersion":1}')
        with self.assertRaisesRegex(ValueError, "duplicate JSON key"):
            artifacts.validate_bundle(self.root, "1")

    def test_real_oras_layout_copy_preserves_native_media_types_and_digest(self):
        for kind in ("images", "types", "recipes"):
            record = next(item for item in self.bundle["artifacts"] if item["kind"] == kind)
            output = self.directory / f"copied-{kind}"
            subprocess.run([
                "oras", "cp", "--from-oci-layout", "--to-oci-layout",
                f"{self.root / record['path']}@{record['digest']}",
                f"{output}:payload",
            ], check=True, capture_output=True)
            artifacts.validate_layout(output, kind, record["digest"])

    def test_results_download_binds_run_and_rejects_non_junit_or_entities(self):
        name = "functional_test_results_corerp-cloud"
        for label, content, expected in [
            ("valid", b'<testsuites><testsuite tests="1"><testcase name="test"/></testsuite></testsuites>', True),
            ("entity", b'<!DOCTYPE x [<!ENTITY a "bad">]><testsuites>&a;</testsuites>', False),
            ("non-junit", b"<html/>", False),
            ("utf16", '<!DOCTYPE x><testsuites/>'.encode("utf-16"), False),
        ]:
            with self.subTest(label=label):
                archive = self.directory / f"{label}.zip"
                with zipfile.ZipFile(archive, "w") as output:
                    output.writestr("processed/results.xml", content)
                metadata = {**self.metadata, "name": name,
                            "size_in_bytes": archive.stat().st_size,
                            "digest": artifacts.digest_file(archive)}

                def download(command, stdout, **kwargs):
                    stdout.write(archive.read_bytes())
                    return subprocess.CompletedProcess(command, 0)

                with patch.dict(os.environ, {"RESULT_ARTIFACT_NAME": name}), \
                        patch.object(artifacts, "api", side_effect=[
                            {"artifacts": [metadata]}, metadata]), \
                        patch.object(artifacts.subprocess, "run", side_effect=download):
                    target = self.directory / f"results-{label}"
                    if expected:
                        artifacts.download(target, results=True)
                        self.assertEqual((target / "processed/results.xml").read_bytes(), content)
                    else:
                        with self.assertRaises(ValueError):
                            artifacts.download(target, results=True)
        with patch.dict(os.environ, {"RESULT_ARTIFACT_NAME": name}), \
                patch.object(artifacts, "api", return_value={"artifacts": []}):
            with self.assertRaisesRegex(ValueError, "missing or ambiguous"):
                artifacts.download(self.directory / "missing-results", results=True)


if __name__ == "__main__":
    unittest.main()
