# Copyright 2026 The Radius Authors.
# Licensed under the Apache License, Version 2.0.

import copy
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest
from unittest.mock import patch
from urllib.error import HTTPError
import zipfile

spec = importlib.util.spec_from_file_location(
    "bicep_types", Path(__file__).with_name("bicep-types.py")
)
publisher = importlib.util.module_from_spec(spec)
spec.loader.exec_module(publisher)

SHA = "a" * 40
ORAS_VERSION_OUTPUT = f"Version: {publisher.pinned_version('ORAS')}\n".encode()
SOURCE = {
    "repository": publisher.REPOSITORY,
    "ref": publisher.REF,
    "commit": SHA,
    "workflow": publisher.WORKFLOW,
    "runId": "123",
    "generationAttempt": 1,
}
ENV = {
    "GITHUB_REPOSITORY": publisher.REPOSITORY,
    "GITHUB_REF": publisher.REF,
    "GITHUB_SHA": SHA,
    "GITHUB_EVENT_NAME": "push",
    "GITHUB_RUN_ID": "123",
    "GITHUB_RUN_ATTEMPT": "1",
    "BICEP_SOURCE_REF_PROTECTED": "true",
    "GITHUB_WORKFLOW_REF": f"{publisher.REPOSITORY}/{publisher.WORKFLOW}@{publisher.REF}",
}


def type_archive(entries=None):
    if entries is None:
        entries = [
            ("index.json", b'{"resources":{"Test/test@v1":{"$ref":"types.json#/0"}}}'),
            ("types.json", b"[]"),
        ]
    buffer = io.BytesIO()
    with tarfile.open(fileobj=buffer, mode="w:gz") as archive:
        for name, data in entries:
            entry = tarfile.TarInfo(name)
            entry.size, entry.mode = len(data), 0o644
            archive.addfile(entry, io.BytesIO(data))
    return buffer.getvalue()


def fixture(directory, source=None, types=None):
    directory = Path(directory)
    layout = directory / "layout"
    blobs = layout / "blobs/sha256"
    blobs.mkdir(parents=True)

    def blob(data, media_type):
        sha = publisher.digest(data)
        (blobs / sha.split(":")[1]).write_bytes(data)
        return {"mediaType": media_type, "digest": sha, "size": len(data)}

    manifest = {
        "schemaVersion": 2,
        "mediaType": publisher.MANIFEST_TYPE,
        "artifactType": publisher.ARTIFACT_TYPE,
        "config": blob(b"{}", publisher.CONFIG_TYPE),
        "layers": [blob(types or type_archive(), publisher.LAYER_TYPE)],
        "annotations": {
            "bicep.serialization.format": "v1",
            "org.opencontainers.image.created": "2026-09-25T00:00:00Z",
        },
    }
    root = blob(json.dumps(manifest).encode(), publisher.MANIFEST_TYPE)
    publisher.write_json(
        layout / "index.json", {"schemaVersion": 2, "manifests": [root]}
    )
    publisher.write_json(layout / "oci-layout", {"imageLayoutVersion": "1.0.0"})
    publisher.write_json(
        directory / "source.json",
        {
            "schemaVersion": 1,
            "extension": "radius",
            "source": source or SOURCE,
            "bicepVersion": publisher.pinned_version("BICEP"),
            "manifestDigest": root["digest"],
        },
    )
    return manifest


def archive_files(files):
    buffer = io.BytesIO()
    with zipfile.ZipFile(buffer, "w") as archive:
        for name, data in files.items():
            archive.writestr(name, data)
    return buffer.getvalue()


def artifact(data, name=None, artifact_id=7):
    return {
        "id": artifact_id,
        "name": name or publisher.bundle_name(SOURCE),
        "size_in_bytes": len(data),
        "digest": publisher.digest(data),
        "expired": False,
        "workflow_run": {"id": 123, "head_branch": "main", "head_sha": SHA},
    }


class FakeGitHub:
    def __init__(self, entries=None, data=None, current_sha=SHA, attempt=1):
        self.entries, self.data, self.current_sha, self.attempt = (
            entries or [],
            data or {},
            current_sha,
            attempt,
        )
        self.calls = []
        self.visibility = "public"
        self.protected = True
        self.run = {
            "id": 123,
            "run_attempt": attempt,
            "event": "push",
            "path": publisher.WORKFLOW,
            "head_branch": "main",
            "head_sha": SHA,
            "repository": {"full_name": publisher.REPOSITORY},
            "head_repository": {"full_name": publisher.REPOSITORY},
        }

    def get(self, path, archive=False):
        self.calls.append(path)
        if path.endswith("/actions/runs/123"):
            return self.run
        if "/artifacts?per_page=100&page=" in path:
            page = int(path.rsplit("=", 1)[1])
            return {
                "total_count": len(self.entries),
                "artifacts": self.entries[(page - 1) * 100 : page * 100],
            }
        if path.endswith("/branches/main"):
            return {"protected": self.protected, "commit": {"sha": self.current_sha}}
        if path.startswith("/orgs/"):
            return {"visibility": self.visibility}
        if archive:
            return self.data[int(path.split("/")[-2])]
        return next(
            entry
            for entry in self.entries
            if path.endswith(f"/artifacts/{entry['id']}")
        )


class PublisherTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.bundle = self.root / "bundle"
        self.manifest = fixture(self.bundle)
        self.data = archive_files(
            {
                path.relative_to(self.bundle).as_posix(): path.read_bytes()
                for path in self.bundle.rglob("*")
                if path.is_file()
            }
        )
        self.artifact = artifact(self.data)
        self.api = FakeGitHub([self.artifact], {7: self.data})
        self.environment = patch.dict(os.environ, ENV, clear=False)
        self.environment.start()
        self.addCleanup(self.environment.stop)
        self.outputs = {}
        output_patch = patch.object(
            publisher,
            "output",
            side_effect=lambda **values: self.outputs.update(values),
        )
        output_patch.start()
        self.addCleanup(output_patch.stop)

    def prepare(self, previous=None, current_sha=SHA, attempt=1):
        self.api.attempt = self.api.run["run_attempt"] = attempt
        self.api.current_sha = current_sha
        os.environ["GITHUB_RUN_ATTEMPT"] = str(attempt)
        if previous is not None:
            raw = archive_files({"receipt.json": json.dumps(previous).encode()})
            entry = artifact(raw, publisher.receipt_name(SOURCE, attempt - 1), 8)
            self.api.entries.append(entry)
            self.api.data[8] = raw
        prepared = self.root / f"prepared-{attempt}"
        publisher.prepare(self.api, "7", self.artifact["digest"], prepared)
        return prepared

    def test_capture_contract_and_deterministic_stamp(self):
        metadata, native = publisher.validate_bundle(self.bundle, SOURCE)
        first = publisher.stamp(self.bundle, metadata, native, self.root / "first")
        second = publisher.stamp(self.bundle, metadata, native, self.root / "second")
        self.assertEqual(first, second)
        final = publisher.read_json(
            self.root / "first/blobs/sha256" / first.split(":")[1]
        )
        self.assertEqual(final["config"], native["config"])
        self.assertEqual(final["layers"], native["layers"])
        self.assertEqual(final["annotations"]["org.opencontainers.image.revision"], SHA)
        self.assertEqual(
            set(final["annotations"]) - set(native["annotations"]),
            {"org.opencontainers.image.source", "org.opencontainers.image.revision"},
        )

    def test_artifact_reuse_and_fresh_run(self):
        with patch.object(publisher, "capture") as generate:
            publisher.select(self.api, self.root / "source")
            self.assertEqual(self.outputs["reuse"], "true")
            self.assertEqual(self.outputs["artifact_id"], 7)
            generate.assert_not_called()
        self.api.entries = []
        publisher.select(self.api, self.root / "source")
        self.assertEqual(publisher.read_json(self.root / "source/source.json"), SOURCE)
        self.assertEqual(self.outputs["reuse"], "false")

    def test_missing_retry_bundle_never_regenerates(self):
        self.api.entries = []
        self.api.run["run_attempt"] = 2
        os.environ["GITHUB_RUN_ATTEMPT"] = "2"
        with self.assertRaisesRegex(ValueError, "Retry bundle is missing"):
            publisher.select(self.api, self.root / "source")

    def test_generation_rejects_nonlocal_or_injected_targets(self):
        for registry in [
            "ghcr.io",
            "biceptypes.azurecr.io",
            "localhost:5000/other",
            "localhost:5000\n--force",
        ]:
            with self.subTest(registry=registry), patch.object(
                publisher, "command"
            ) as tool, self.assertRaises(ValueError):
                publisher.capture(
                    "source.json", "index.json", registry, self.root / "capture"
                )
            tool.assert_not_called()

    def test_rejected_contexts(self):
        for key, value in [
            ("GITHUB_REPOSITORY", "fork/radius"),
            ("GITHUB_REF", "refs/tags/v1.0.0"),
            ("GITHUB_EVENT_NAME", "pull_request"),
            ("GITHUB_EVENT_NAME", "merge_group"),
            ("GITHUB_EVENT_NAME", "pull_request_target"),
            ("GITHUB_RUN_ID", "0"),
            ("GITHUB_RUN_ATTEMPT", "-1"),
            ("GITHUB_SHA", "main"),
            ("GITHUB_SHA", "0" * 40),
            ("BICEP_SOURCE_REF_PROTECTED", "false"),
            ("GITHUB_WORKFLOW_REF", "untrusted-workflow"),
        ]:
            with self.subTest(key=key, value=value), patch.dict(
                os.environ, {key: value}
            ):
                with self.assertRaises(ValueError):
                    publisher.source_context(self.api)
        for key, value in [
            ("head_sha", "b" * 40),
            ("path", ".github/workflows/build-validation.yaml"),
            ("head_branch", "topic"),
            ("run_attempt", 2),
            ("event", "pull_request"),
        ]:
            with self.subTest(key=key), patch.dict(self.api.run, {key: value}):
                with self.assertRaises(ValueError):
                    publisher.source_context(self.api)

    def test_manual_main_is_verified(self):
        with patch.dict(os.environ, {"GITHUB_EVENT_NAME": "workflow_dispatch"}):
            self.api.run["event"] = "workflow_dispatch"
            self.assertEqual(publisher.source_context(self.api), SOURCE)

    def test_artifact_identity_rejections(self):
        for changes in [
            {"expired": True},
            {"name": "PR-bundle"},
            {"digest": None},
            {"size_in_bytes": publisher.MAX_BYTES + 1},
            {"id": False},
            {"workflow_run": {"id": 999, "head_branch": "main", "head_sha": SHA}},
            {"workflow_run": {"id": 123, "head_branch": "main", "head_sha": "b" * 40}},
        ]:
            with self.subTest(changes=changes), self.assertRaises(ValueError):
                publisher.validate_artifact(
                    {**self.artifact, **changes}, SOURCE, publisher.bundle_name(SOURCE)
                )
        self.api.entries = [self.artifact, self.artifact]
        with self.assertRaisesRegex(ValueError, "Ambiguous"):
            publisher.select(self.api, self.root / "source")

    def test_failed_lookup_is_not_absence(self):
        with patch.object(self.api, "get", side_effect=RuntimeError("HTTP 403")):
            with self.assertRaisesRegex(RuntimeError, "403"):
                publisher.select(self.api, self.root / "source")
        for response in [
            {"total_count": 1001, "artifacts": []},
            {"total_count": 1, "artifacts": []},
        ]:
            with self.subTest(response=response), patch.object(
                self.api, "get", return_value=response
            ):
                with self.assertRaises(ValueError):
                    publisher.artifacts(self.api, SOURCE)

    def test_digest_mismatch_is_fatal_before_extraction(self):
        bad = {**self.artifact, "digest": "sha256:" + "f" * 64}
        with self.assertRaisesRegex(ValueError, "archive digest"):
            publisher.extract_artifact(self.api, bad, self.root / "download")
        self.assertFalse((self.root / "download").exists())

    def test_unsafe_zip_files(self):
        for name in ["../escape", "/absolute", "a\\b", "a/../escape"]:
            data = archive_files({name: b"bad"})
            api = FakeGitHub(data={7: data})
            with self.subTest(name=name), self.assertRaisesRegex(ValueError, "Unsafe"):
                publisher.extract_artifact(api, artifact(data), self.root / "download")
        for mode in [0o120777, 0o100755, 0o010644]:
            buffer = io.BytesIO()
            with zipfile.ZipFile(buffer, "w") as archive:
                entry = zipfile.ZipInfo("unsafe")
                entry.external_attr = mode << 16
                archive.writestr(entry, b"payload")
            data = buffer.getvalue()
            with self.subTest(mode=mode), self.assertRaises(ValueError):
                publisher.extract_artifact(
                    FakeGitHub(data={7: data}), artifact(data), self.root / "download"
                )

    def test_source_substitution(self):
        for changes in [
            {"commit": "b" * 40},
            {"repository": "fork/radius"},
            {"ref": "refs/pull/5/merge"},
            {"runId": "999"},
            {"generationAttempt": 2},
            {"registry_target": "attacker"},
        ]:
            metadata = publisher.read_json(self.bundle / "source.json")
            metadata["source"] = {**SOURCE, **changes}
            publisher.write_json(self.bundle / "source.json", metadata)
            with self.subTest(changes=changes), self.assertRaises(ValueError):
                publisher.validate_bundle(self.bundle, SOURCE)

    def test_unexpected_and_executable_bundle_files(self):
        for name in ["run.sh", "Makefile", "layout/blobs/sha256/" + "f" * 64]:
            path = self.bundle / name
            path.write_text("untrusted")
            with self.subTest(name=name), self.assertRaises(ValueError):
                publisher.validate_bundle(self.bundle, SOURCE)
            path.unlink()
        path = self.bundle / "source.json"
        path.chmod(0o755)
        with self.assertRaisesRegex(ValueError, "Executable"):
            publisher.validate_bundle(self.bundle, SOURCE)

    def test_invalid_descriptors_and_binary_layers(self):
        for modify in [
            lambda m: m.update(artifactType="application/unknown"),
            lambda m: m["config"].update(mediaType="application/json"),
            lambda m: m["config"].update(urls=["https://attacker.invalid/"]),
            lambda m: m["layers"][0].update(
                mediaType="application/vnd.ms.bicep.provider.layer.v1.linux-x64.binary"
            ),
            lambda m: m["layers"].append(m["layers"][0]),
            lambda m: m["layers"][0].update(size=999),
            lambda m: m["annotations"].update(
                {"org.opencontainers.image.revision": "attacker"}
            ),
        ]:
            manifest = copy.deepcopy(self.manifest)
            modify(manifest)
            raw = json.dumps(manifest).encode()
            sha = publisher.digest(raw)
            path = self.bundle / "layout/blobs/sha256" / sha.split(":")[1]
            path.write_bytes(raw)
            metadata = publisher.read_json(self.bundle / "source.json")
            metadata["manifestDigest"] = sha
            publisher.write_json(self.bundle / "source.json", metadata)
            publisher.write_json(
                self.bundle / "layout/index.json",
                {
                    "schemaVersion": 2,
                    "manifests": [
                        {
                            "digest": sha,
                            "size": len(raw),
                            "mediaType": publisher.MANIFEST_TYPE,
                        }
                    ],
                },
            )
            with self.assertRaises(ValueError):
                publisher.validate_bundle(self.bundle, SOURCE)
            path.unlink()

    def test_unsafe_types_archives_and_limits(self):
        for entries in [
            [("../escape.json", b"{}")],
            [("index.json", b"not-json")],
            [("run.sh", b"echo untrusted")],
            [("index.json", b'{"resources":{"a":{"$ref":"missing.json#/0"}}}')],
            [("index.json", b'{"resources":{}}'), ("index.json", b'{"resources":{}}')],
        ]:
            with self.subTest(entries=entries), self.assertRaises(ValueError):
                publisher.validate_types(type_archive(entries))
        data = type_archive()
        with patch.object(publisher, "MAX_EXPANDED_BYTES", 1), self.assertRaisesRegex(
            ValueError, "limits"
        ):
            publisher.validate_types(data)
        with patch.object(publisher, "MAX_FILES", 1), self.assertRaisesRegex(
            ValueError, "limits"
        ):
            publisher.validate_types(data)

    def test_exact_archive_size_boundary_and_strict_json(self):
        entries = [("index.json", b'{"resources":{}}')]
        data = type_archive(entries)
        with patch.object(publisher, "MAX_EXPANDED_BYTES", len(entries[0][1])):
            publisher.validate_types(data)
        with patch.object(publisher, "MAX_EXPANDED_BYTES", len(entries[0][1]) - 1):
            with self.assertRaisesRegex(ValueError, "limits"):
                publisher.validate_types(data)
        for malformed in [b'{"a":1,"a":2}', b'{"a":NaN}', b'{"a":Infinity}']:
            with self.subTest(data=malformed), self.assertRaises(ValueError):
                publisher.decode_json(malformed)

    def test_api_retry_deadline_and_failure_are_bounded(self):
        with patch.dict(os.environ, {"GH_TOKEN": "fake-test-token"}):
            api = publisher.GitHub()
        opener = publisher.build_opener(publisher.NoRedirect())
        for status, attempts in [(401, 1), (403, 1), (404, 1), (503, 3)]:
            with self.subTest(status=status), patch.object(
                publisher, "build_opener", return_value=opener
            ), patch.object(
                opener,
                "open",
                side_effect=HTTPError("url", status, "failure", {}, None),
            ) as request, patch.object(
                publisher.time, "sleep"
            ), self.assertRaises(
                RuntimeError
            ):
                api.get(f"/repos/{publisher.REPOSITORY}/actions/runs/123")
            self.assertEqual(request.call_count, attempts)
            self.assertTrue(
                all(call.kwargs["timeout"] <= 20 for call in request.call_args_list)
            )
        api.deadline = 0
        with patch.object(publisher, "build_opener") as opener, self.assertRaisesRegex(
            ValueError, "deadline"
        ):
            api.get(f"/repos/{publisher.REPOSITORY}/actions/runs/123")
        opener.assert_not_called()

    def test_download_redirect_does_not_forward_repository_token(self):
        with patch.dict(os.environ, {"GH_TOKEN": "fake-test-token"}):
            api = publisher.GitHub()
        location = "https://artifact.example.invalid/signed-download"
        opener = publisher.build_opener(publisher.NoRedirect())
        redirect = HTTPError("url", 302, "redirect", {"Location": location}, None)
        with patch.object(publisher, "build_opener", return_value=opener), patch.object(
            opener, "open", side_effect=redirect
        ), patch.object(
            publisher, "urlopen", return_value=io.BytesIO(self.data)
        ) as download:
            self.assertEqual(
                api.get(
                    f"/repos/{publisher.REPOSITORY}/actions/artifacts/7/zip",
                    archive=True,
                ),
                self.data,
            )
        self.assertEqual(download.call_args.args, (location,))
        self.assertEqual(set(download.call_args.kwargs), {"timeout"})
        with self.assertRaises(ValueError):
            api._read(io.BytesIO(b"1234"), 3)
        self.assertEqual(api._read(io.BytesIO(b"123"), 3), b"123")

    def test_prepare_accepts_upload_action_digest_and_reuses_bundle_bytes(self):
        prepared = self.root / "prepared"
        publisher.prepare(
            self.api, "7", self.artifact["digest"].split(":")[1], prepared
        )
        receipt = publisher.read_json(prepared / "receipt.json")
        self.assertEqual(receipt["bundle"]["artifactDigest"], self.artifact["digest"])
        self.assertEqual(receipt["source"], SOURCE)

    def test_pair_finishes_if_main_advances_after_first_write(self):
        prepared = self.prepare()

        def copying(*args, **kwargs):
            self.api.current_sha = "b" * 40

        with patch.object(
            publisher, "command", return_value=ORAS_VERSION_OUTPUT
        ), patch.object(
            publisher, "copy_exact", side_effect=copying
        ) as copier, patch.object(
            publisher, "verify_manifest", return_value=b"same bytes"
        ):
            publisher.publish(self.api, prepared)
        self.assertEqual(copier.call_count, 2)
        self.assertEqual(copier.call_args_list[0].args[1], publisher.GHCR)
        self.assertEqual(copier.call_args_list[1].args[1], publisher.ACR)
        self.assertIn("@sha256:", copier.call_args_list[1].args[0])
        receipt = publisher.read_json(prepared / "receipt.json")
        self.assertEqual(receipt["status"], "published")
        self.assertEqual(
            receipt["destinations"]["ghcr"]["digest"],
            receipt["destinations"]["acr"]["digest"],
        )
        self.assertEqual(self.outputs["status"], "published")

    def test_clean_stale_snapshot_never_mutates(self):
        prepared = self.prepare(current_sha="b" * 40)
        with patch.object(
            publisher, "command", return_value=ORAS_VERSION_OUTPUT
        ), patch.object(publisher, "copy_exact") as copier:
            publisher.publish(self.api, prepared)
        copier.assert_not_called()
        self.assertEqual(self.outputs["status"], "superseded")

    def test_missing_progress_on_superseded_retry_fails(self):
        prepared = self.prepare(attempt=2, current_sha="b" * 40)
        with patch.object(
            publisher, "command", return_value=ORAS_VERSION_OUTPUT
        ), patch.object(publisher, "copy_exact") as copier, self.assertRaisesRegex(
            ValueError, "unknown"
        ):
            publisher.publish(self.api, prepared)
        copier.assert_not_called()
        self.assertNotIn("status", self.outputs)

    def test_partial_pair_is_not_success_and_retries_reuse_the_digest(self):
        first = self.prepare()
        with patch.object(
            publisher, "command", return_value=ORAS_VERSION_OUTPUT
        ), patch.object(
            publisher, "copy_exact", side_effect=[None, RuntimeError("ACR unavailable")]
        ), self.assertRaisesRegex(
            RuntimeError, "ACR unavailable"
        ):
            publisher.publish(self.api, first)
        partial = publisher.read_json(first / "receipt.json")
        self.assertEqual(partial["status"], "partial")
        self.assertEqual(partial["phase"], "writing-acr")
        second = self.prepare(previous=partial, current_sha="b" * 40, attempt=2)
        self.assertEqual(
            publisher.read_json(second / "receipt.json")["status"], "partial"
        )
        with patch.object(
            publisher, "command", return_value=ORAS_VERSION_OUTPUT
        ), patch.object(
            publisher, "verify_manifest", return_value=b"same"
        ), patch.object(
            publisher, "copy_exact"
        ) as copier:
            publisher.publish(self.api, second)
        self.assertEqual(copier.call_count, 1)
        self.assertEqual(copier.call_args.args[1], publisher.ACR)
        self.assertEqual(
            publisher.read_json(second / "receipt.json")["manifestDigest"],
            partial["manifestDigest"],
        )

    def test_newer_pair_is_not_rolled_back_or_called_superseded(self):
        first = self.prepare()
        partial = publisher.read_json(first / "receipt.json")
        partial.update(status="partial", phase="writing-ghcr")
        second = self.prepare(previous=partial, current_sha="b" * 40, attempt=2)
        with patch.object(
            publisher, "command", return_value=ORAS_VERSION_OUTPUT
        ), patch.object(
            publisher, "verify_manifest", side_effect=ValueError("newer digest")
        ), patch.object(
            publisher, "copy_exact"
        ) as copier, self.assertRaisesRegex(
            ValueError, "newer"
        ):
            publisher.publish(self.api, second)
        copier.assert_not_called()
        self.assertEqual(
            publisher.read_json(second / "receipt.json")["status"], "partial"
        )
        self.assertNotIn("status", self.outputs)

    def test_partial_progress_survives_a_credential_failure_before_publish(self):
        first = self.prepare()
        partial = publisher.read_json(first / "receipt.json")
        partial.update(status="partial", phase="writing-ghcr")
        second = self.prepare(previous=partial, attempt=2)
        self.assertEqual(
            publisher.read_json(second / "receipt.json")["phase"], "writing-ghcr"
        )
        with patch.dict(
            os.environ, {"GITHUB_STEP_SUMMARY": str(self.root / "summary")}
        ):
            publisher.report(second)
        self.assertIn("**partial**", (self.root / "summary").read_text())
        self.assertIn("not verified", (self.root / "summary").read_text())

    def test_visibility_protection_and_lookup_errors_never_publish(self):
        for setting, value in [("visibility", "private"), ("protected", False)]:
            prepared = self.prepare()
            setattr(self.api, setting, value)
            with patch.object(
                publisher, "command", return_value=ORAS_VERSION_OUTPUT
            ), patch.object(publisher, "copy_exact") as copier, self.assertRaises(
                ValueError
            ):
                publisher.publish(self.api, prepared)
            copier.assert_not_called()
            setattr(self.api, setting, "public" if setting == "visibility" else True)
            import shutil

            shutil.rmtree(prepared)

    def test_copy_reconciliation_and_no_auth_failure_fallback(self):
        with patch.object(
            publisher, "command", side_effect=RuntimeError("HTTP 503")
        ), patch.object(publisher, "verify_manifest") as verify:
            publisher.copy_exact("source@sha256:x", "target:edge", "sha256:x")
            verify.assert_called_once()
        with patch.object(
            publisher, "command", side_effect=RuntimeError("HTTP 401")
        ), patch.object(publisher, "verify_manifest") as verify, self.assertRaisesRegex(
            RuntimeError, "401"
        ):
            publisher.copy_exact("source@sha256:x", "target:edge", "sha256:x")
        verify.assert_not_called()
        with patch.object(
            publisher, "command", side_effect=subprocess.TimeoutExpired("oras", 180)
        ) as call, patch.object(
            publisher, "verify_manifest", side_effect=RuntimeError("HTTP 503")
        ), patch.object(
            publisher.time, "sleep"
        ), self.assertRaises(
            subprocess.TimeoutExpired
        ):
            publisher.copy_exact("source@sha256:x", "target:edge", "sha256:x")
        self.assertEqual(call.call_count, 3)

    def test_summary_truth_table(self):
        for mode, legacy, direct, status in [
            ("skip", "skipped", "skipped", ""),
            ("legacy", "success", "skipped", ""),
            ("direct", "skipped", "success", "published"),
            ("direct", "skipped", "success", "superseded"),
        ]:
            publisher.summary(mode, "success", legacy, direct, status)
        for args in [
            ("", "success", "skipped", "skipped", ""),
            ("skip", "failure", "skipped", "skipped", ""),
            ("skip", "success", "success", "skipped", ""),
            ("legacy", "success", "skipped", "skipped", ""),
            ("legacy", "success", "success", "success", "published"),
            ("direct", "success", "skipped", "skipped", ""),
            ("direct", "success", "skipped", "failure", "partial"),
            ("direct", "success", "skipped", "success", ""),
        ]:
            with self.subTest(args=args), self.assertRaises(ValueError):
                publisher.summary(*args)

    def test_tool_pin_must_match_the_complete_version(self):
        with patch.object(publisher, "command", return_value=ORAS_VERSION_OUTPUT):
            publisher.verify_tool("ORAS")
        for output in [
            b"Version: 1.3.40\n",
            b"Version: 1.3.4-untrusted\n",
            b"unversioned",
        ]:
            with self.subTest(output=output), patch.object(
                publisher, "command", return_value=output
            ):
                with self.assertRaisesRegex(ValueError, "Pinned"):
                    publisher.verify_tool("ORAS")


if __name__ == "__main__":
    unittest.main()
