#!/usr/bin/env python3
"""Cross-platform release automation for radius-project/radius.

Works on Windows, Linux, and macOS. Requires only:
  - Python 3.14+
  - gh CLI (authenticated)
  - git CLI (only for specific operations)

No third-party Python packages required.
"""

from __future__ import annotations

import argparse
import base64
import json
import re
import shutil
import subprocess
import sys
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Callable, NoReturn

# =============================================================================
# Constants
# =============================================================================

RADIUS_GH_REPO = "radius-project/radius"
DE_GH_REPO = "azure-octo/deployment-engine"
DOCS_GH_REPO = "radius-project/docs"
SAMPLES_GH_REPO = "radius-project/samples"

# =============================================================================
# Output helpers
# =============================================================================


def _color(text: str, code: str) -> str:
    return f"\033[{code}m{text}\033[0m"


def log(msg: str) -> None:
    print(f"[INFO] {msg}")


def debug(msg: str) -> None:
    print(_color(f"[DEBUG] {msg}", "35"))


def warn(msg: str) -> None:
    print(_color(f"[WARN] {msg}", "33"), file=sys.stderr)


def error(msg: str) -> NoReturn:
    print(_color(f"[ERROR] {msg}", "31"), file=sys.stderr)
    sys.exit(1)


# =============================================================================
# Configuration
# =============================================================================


@dataclass
class Config:
    mode: str = "rc"
    version: str = ""
    username: str = ""
    user_name: str = ""  # GitHub display name (for Signed-off-by)
    user_email: str = ""  # GitHub email (for Signed-off-by)
    radius_repo: str = "."
    base_branch: str = "main"
    release_branch: str = ""
    work_branch: str = ""
    bugfix_commit: str = ""
    version_commit: str = ""
    release_notes_file: str = ""
    release_notes_repo_path: str = ""
    resolved_release_notes_repo_path: str = ""
    run_radius_verification: bool = False
    auto_merge_pr: bool = False
    assume_yes: bool = False
    execute_mutations: bool = False


cfg = Config()

# =============================================================================
# Subprocess helpers
# =============================================================================


def run_cmd(
    args: list[str],
    *,
    capture: bool = True,
    check: bool = True,
    cwd: str | Path | None = None,
    input_str: str | None = None,
) -> subprocess.CompletedProcess[str]:
    """Run a subprocess command, returning CompletedProcess."""
    result = subprocess.run(
        args,
        capture_output=capture,
        encoding="utf-8",
        errors="replace",
        check=False,
        cwd=cwd,
        input=input_str,
    )
    if check and result.returncode != 0:
        detail = (
            result.stderr.strip() if result.stderr else f"exit code {result.returncode}"
        )
        error(f"Command failed: {shlex_join(args)}\n  {detail}")
    return result


def shlex_join(args: list[str]) -> str:
    """Join args into a shell-like display string (for logging)."""
    return subprocess.list2cmdline(args)


def handle_existing_pr_state(
    *,
    step_label: str,
    pr_info: dict,
    merged_message: str | None = None,
    open_message: str | None = None,
    other_message: str | None = None,
) -> str:
    """Normalize handling of existing PR states.

    Returns one of: "merged", "open", "other".
    """
    state = pr_info.get("state", "").upper()
    url = pr_info.get("url", "")

    if state == "MERGED":
        warn(merged_message or f"SKIP {step_label}: PR already merged: {url}")
        return "merged"
    if state == "OPEN":
        warn(open_message or f"SKIP {step_label}: PR already open: {url}")
        return "open"

    warn(other_message or f"SKIP {step_label}: PR exists in state '{state}': {url}")
    return "other"


def require_command(cmd: str) -> None:
    """Verify a CLI tool is available on PATH."""
    if shutil.which(cmd) is None:
        error(f"Required command not found: {cmd}")


# =============================================================================
# Execution control
# =============================================================================


def confirm_or_exit(prompt: str) -> None:
    """Ask for user confirmation. Skipped when --yes is set."""
    if cfg.assume_yes:
        return
    try:
        answer = input(f"{prompt} [y/N]: ").strip()
    except (EOFError, KeyboardInterrupt):
        print()
        error("Operation cancelled")
    if answer.lower() not in ("y", "yes"):
        error("Operation cancelled by user")


def run_readonly(
    args: list[str], *, cwd: str | Path | None = None
) -> subprocess.CompletedProcess[str]:
    """Run a read-only command with logging."""
    debug(f"RUN: {shlex_join(args)}")
    return run_cmd(args, cwd=cwd)


def run_mutating(
    description: str, args: list[str], *, cwd: str | Path | None = None
) -> subprocess.CompletedProcess[str] | None:
    """Guard a local command behind --execute.

    Logs the full command in dry-run mode (appropriate for local git
    operations where the raw command is meaningful).
    """
    if not cfg.execute_mutations:
        log(f"DRY-RUN (mutation blocked): {description}")
        debug(f"DRY-RUN CMD: {shlex_join(args)}")
        return None

    confirm_or_exit(f"Proceed with: {description}?")
    debug(f"RUN: {shlex_join(args)}")
    return run_cmd(args, cwd=cwd)


def run_mutating_api(
    description: str, fn: Callable[..., Any], *args: Any, **kwargs: Any
) -> Any:
    """Guard an API-based mutation behind --execute.

    Only logs the description (not raw command) to avoid dumping large
    payloads like file contents in dry-run output.
    """
    if not cfg.execute_mutations:
        log(f"DRY-RUN (mutation blocked): {description}")
        return None

    confirm_or_exit(f"Proceed with: {description}?")
    debug(f"EXEC: {description}")
    return fn(*args, **kwargs)


# =============================================================================
# Re-run command builder
# =============================================================================


# Mapping of Config attribute -> CLI flag for re-run command reconstruction.
# Adding a new Config field only requires a new entry here.
_RERUN_FLAGS: list[tuple[str, str]] = [
    ("mode", "--mode"),
    ("version", "--version"),
    ("username", "--username"),
    ("radius_repo", "--radius-repo"),
    ("base_branch", "--base-branch"),
    ("release_branch", "--release-branch"),
    ("work_branch", "--work-branch"),
    ("release_notes_file", "--release-notes-file"),
    ("release_notes_repo_path", "--release-notes-repo-path"),
    ("bugfix_commit", "--bugfix-commit"),
    ("version_commit", "--version-commit"),
    ("run_radius_verification", "--run-radius-verification"),
    ("auto_merge_pr", "--auto-merge-pr"),
]

# Attributes whose non-empty defaults should be suppressed in the re-run command.
_RERUN_DEFAULTS: dict[str, str] = {"base_branch": "main", "radius_repo": "."}


def _build_rerun_command() -> str:
    """Reconstruct the full CLI command to re-run the current release.

    Always includes --execute --yes for unattended resume.  The returned
    string is copy-pasteable into a terminal.  New Config fields only need
    an entry in ``_RERUN_FLAGS`` to be included automatically.
    """
    parts: list[str] = [sys.executable, sys.argv[0]]
    for attr, flag in _RERUN_FLAGS:
        val = getattr(cfg, attr)
        default = _RERUN_DEFAULTS.get(attr, False if isinstance(val, bool) else "")
        if val and val != default:
            if isinstance(val, bool):
                parts.append(flag)
            else:
                parts.extend([flag, str(val)])
    parts.extend(["--execute", "--yes"])
    return shlex_join(parts)


# =============================================================================
# Version helpers
# =============================================================================


def normalize_version(value: str) -> str:
    """Strip leading 'v' from a version string."""
    return value.removeprefix("v")


def channel_from_version(version: str) -> str:
    """Extract major.minor channel from version string."""
    parts = version.split(".")
    return f"{parts[0]}.{parts[1]}"


def resolve_repo_relative_path(path: str, *, flag_name: str) -> str:
    """Normalize and validate a repository-relative path."""
    normalized = path.strip().replace("\\", "/")
    while normalized.startswith("./"):
        normalized = normalized[2:]

    if not normalized:
        error(f"{flag_name} cannot be empty")

    if normalized.startswith("/") or re.match(r"^[A-Za-z]:", normalized):
        error(f"{flag_name} must be repository-relative (not absolute): {path}")

    return normalized


def resolve_release_notes_repo_path() -> str:
    """Resolve the repository path for release notes commits."""
    candidate = cfg.release_notes_repo_path or cfg.release_notes_file
    return resolve_repo_relative_path(candidate, flag_name="--release-notes-repo-path")


def validate_version_format() -> None:
    """Validate cfg.version matches expected format for the current mode."""
    pattern = r"^\d+\.\d+\.\d+(-rc\d+)?$"
    if not re.match(pattern, cfg.version):
        error("Invalid --version. Expected X.Y.Z or X.Y.Z-rcN")

    match cfg.mode:
        case "rc":
            if not re.search(r"-rc\d+$", cfg.version):
                error("RC mode requires version like X.Y.Z-rcN")
        case "final" | "patch":
            if "-rc" in cfg.version:
                error(f"{cfg.mode} mode requires stable version X.Y.Z")


def infer_mode_from_version(version: str) -> str:
    """Infer release mode from version string.

    - X.Y.Z-rcN  -> rc
    - X.Y.Z with Z > 0 -> patch
    - X.Y.0      -> final
    """
    if re.search(r"-rc\d+$", version):
        return "rc"
    parts = version.split(".")
    if len(parts) >= 3:
        patch_num = int(parts[2].split("-")[0])
        if patch_num > 0:
            return "patch"
    return "final"


# =============================================================================
# Simple YAML parser / emitter for versions.yaml
#
# Handles the specific format used by versions.yaml:
#   supported:
#     - channel: '0.55'
#       version: 'v0.55.0-rc4'
#   deprecated:
#     - channel: '0.53'
#       version: 'v0.53.0'
#
# This eliminates the need for yq on all platforms.
# =============================================================================


def _yaml_unquote(s: str) -> str:
    """Remove surrounding YAML quotes from a value."""
    s = s.strip()
    if len(s) >= 2 and s[0] == s[-1] and s[0] in ("'", '"'):
        return s[1:-1]
    return s


def parse_versions_yaml(content: str) -> dict[str, list[dict[str, str]]]:
    """Parse versions.yaml into {"supported": [...], "deprecated": [...]}."""
    result: dict[str, list[dict[str, str]]] = {"supported": [], "deprecated": []}
    current_section: str | None = None
    current_item: dict[str, str] = {}

    for raw_line in content.splitlines():
        line = raw_line.rstrip()
        stripped = line.strip()

        # Skip blanks and comments
        if not stripped or stripped.startswith("#"):
            continue

        # Top-level key (e.g., "supported:" or "deprecated:")
        if not line[0].isspace() and stripped.endswith(":"):
            # Flush pending item
            if current_item and current_section:
                result[current_section].append(current_item)
                current_item = {}
            key = stripped[:-1].strip()
            current_section = key if key in result else None
            continue

        if current_section is None:
            continue

        # List item start: "  - key: value"
        if stripped.startswith("- "):
            # Flush previous item in this section
            if current_item:
                result[current_section].append(current_item)
                current_item = {}
            rest = stripped[2:]
            if ":" in rest:
                k, v = rest.split(":", 1)
                current_item[k.strip()] = _yaml_unquote(v)
            continue

        # Continuation key-value: "    key: value"
        if ":" in stripped:
            k, v = stripped.split(":", 1)
            current_item[k.strip()] = _yaml_unquote(v)

    # Flush last item
    if current_item and current_section:
        result[current_section].append(current_item)

    return result


def emit_versions_yaml(data: dict[str, list[dict[str, str]]]) -> str:
    """Emit versions.yaml content in canonical format."""
    lines: list[str] = []
    for section in ("supported", "deprecated"):
        if section not in data:
            continue
        lines.append(f"{section}:")
        for item in data[section]:
            first = True
            for k, v in item.items():
                prefix = "- " if first else "  "
                lines.append(f"  {prefix}{k}: '{v}'")
                first = False
    return "\n".join(lines) + "\n"


# =============================================================================
# GitHub API helpers (uses gh CLI + Python json, no jq needed)
# =============================================================================


def gh_api_json(
    endpoint: str,
    *,
    method: str = "GET",
    body: dict[str, Any] | None = None,
    silent_fail: bool = False,
) -> Any:
    """Call gh api and return parsed JSON (or None on silent_fail)."""
    cmd = ["gh", "api", endpoint]
    if method != "GET":
        cmd.extend(["-X", method])

    input_str: str | None = None
    if body is not None:
        cmd.extend(["--input", "-"])
        input_str = json.dumps(body)

    result = run_cmd(cmd, check=not silent_fail, input_str=input_str)
    if silent_fail and result.returncode != 0:
        return None

    stdout = result.stdout.strip()
    if not stdout:
        return None
    try:
        return json.loads(stdout)
    except json.JSONDecodeError:
        if silent_fail:
            return None
        raise


def gh_verify_repo(repo: str) -> None:
    result = run_cmd(["gh", "repo", "view", repo, "--json", "name"], check=False)
    if result.returncode != 0:
        error(f"Cannot access GitHub repository: {repo}")


def gh_assert_branch_exists(repo: str, branch: str) -> None:
    data = gh_api_json(f"repos/{repo}/branches/{branch}", silent_fail=True)
    if data is None:
        error(f"Remote branch not found: {branch} in {repo}")


def gh_assert_branch_not_exists(repo: str, branch: str) -> None:
    data = gh_api_json(f"repos/{repo}/branches/{branch}", silent_fail=True)
    if data is not None:
        error(f"Remote branch already exists: {branch} in {repo}")


def gh_assert_tag_not_exists(repo: str, tag: str) -> None:
    data = gh_api_json(f"repos/{repo}/git/ref/tags/{tag}", silent_fail=True)
    if data is not None:
        error(f"Tag already exists in remote: {tag} in {repo}")


def gh_require_commit(repo: str, sha: str) -> None:
    data = gh_api_json(f"repos/{repo}/git/commits/{sha}", silent_fail=True)
    if data is None:
        error(f"Commit not found in {repo}: {sha}")


def gh_get_branch_sha(repo: str, branch: str) -> str:
    data = gh_api_json(f"repos/{repo}/git/refs/heads/{branch}")
    return data["object"]["sha"]


def gh_create_branch_from_ref(repo: str, new_branch: str, base_branch: str) -> None:
    """Create a new branch pointing at the tip of an existing branch."""
    sha = gh_get_branch_sha(repo, base_branch)
    gh_api_json(
        f"repos/{repo}/git/refs",
        method="POST",
        body={"ref": f"refs/heads/{new_branch}", "sha": sha},
    )
    log(f"Branch created: https://github.com/{repo}/tree/{new_branch}")


def gh_get_file_content(
    repo: str, path: str, ref: str, *, silent_fail: bool = False
) -> str | None:
    """Fetch a file's content from a GitHub repo at a given ref.

    Returns *None* when *silent_fail* is True and the file is not found.
    """
    data = gh_api_json(
        f"repos/{repo}/contents/{path}?ref={ref}", silent_fail=silent_fail
    )
    if data is None:
        return None
    # GitHub returns base64 with line breaks; clean before decoding
    content_b64 = data["content"].replace("\n", "")
    return base64.b64decode(content_b64).decode("utf-8")


# ---------------------------------------------------------------------------
# State-checking helpers (used for resume logic)
# ---------------------------------------------------------------------------


def gh_tag_exists(repo: str, tag: str) -> bool:
    """Check whether a tag exists in a remote repository."""
    data = gh_api_json(f"repos/{repo}/git/ref/tags/{tag}", silent_fail=True)
    return data is not None


def gh_branch_exists(repo: str, branch: str) -> bool:
    """Check whether a branch exists in a remote repository."""
    data = gh_api_json(f"repos/{repo}/branches/{branch}", silent_fail=True)
    return data is not None


def gh_find_pr(repo: str, base: str, head: str) -> dict | None:
    """Find the most recent PR for *head* -> *base* in any state.

    Returns a dict with keys ``number``, ``state``, ``url`` or *None*.
    ``state`` is one of OPEN, CLOSED, MERGED (uppercase).
    """
    result = run_cmd(
        [
            "gh",
            "pr",
            "list",
            "--repo",
            repo,
            "--base",
            base,
            "--head",
            head,
            "--state",
            "all",
            "--json",
            "number,state,url",
            "--limit",
            "1",
        ]
    )
    prs = json.loads(result.stdout.strip() or "[]")
    return prs[0] if prs else None


def gh_get_pr_merge_commit_sha(repo: str, pr_number: int) -> str | None:
    """Get the merge commit SHA of a merged PR via the REST API."""
    data = gh_api_json(f"repos/{repo}/pulls/{pr_number}", silent_fail=True)
    if data is None:
        return None
    return data.get("merge_commit_sha")


def gh_versions_yaml_has_version(repo: str, ref: str, version: str) -> bool:
    """Check if versions.yaml at *ref* already lists *version* as supported."""
    file_content = gh_get_file_content(repo, "versions.yaml", ref, silent_fail=True)
    if file_content is None:
        return False
    try:
        channel = channel_from_version(version)
        target = f"v{version}"
        parsed = parse_versions_yaml(file_content)
        return any(
            e.get("channel") == channel and e.get("version") == target
            for e in parsed.get("supported", [])
        )
    except Exception:
        return False


def gh_release_exists(repo: str, tag: str) -> bool:
    """Check whether a GitHub Release exists for *tag*."""
    data = gh_api_json(f"repos/{repo}/releases/tags/{tag}", silent_fail=True)
    return data is not None


def _find_workflow_run(
    repo: str,
    workflow_file: str,
    *,
    predicate: Callable[[dict], bool],
) -> dict | None:
    """Return the first recent workflow run matching *predicate*, or *None*."""
    result = run_cmd(
        [
            "gh",
            "run",
            "list",
            "--repo",
            repo,
            "--workflow",
            workflow_file,
            "--json",
            "status,conclusion,headBranch,event,url",
            "--limit",
            "25",
        ]
    )
    runs = json.loads(result.stdout.strip() or "[]")
    return next((r for r in runs if predicate(r)), None)


def gh_get_workflow_run_for_tag(repo: str, workflow_file: str, tag: str) -> dict | None:
    """Find the most recent workflow run triggered by *tag*."""
    return _find_workflow_run(
        repo,
        workflow_file,
        predicate=lambda r: r.get("headBranch") == tag,
    )


def gh_get_workflow_dispatch_run_for_ref(
    repo: str,
    workflow_file: str,
    ref: str,
) -> dict | None:
    """Find the most recent workflow_dispatch run for *ref*."""
    return _find_workflow_run(
        repo,
        workflow_file,
        predicate=lambda r: (
            r.get("event") == "workflow_dispatch" and r.get("headBranch") == ref
        ),
    )


def wait_for_dispatched_run_url(
    repo: str,
    workflow_file: str,
    ref: str,
    *,
    timeout_seconds: int = 40,
    poll_interval_seconds: int = 2,
) -> str | None:
    """Wait for a newly dispatched workflow run to appear and return its URL."""
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        run = gh_get_workflow_dispatch_run_for_ref(repo, workflow_file, ref)
        if run and run.get("url"):
            return run["url"]
        time.sleep(poll_interval_seconds)
    return None


def previous_channel_branch(version: str) -> str:
    """Compute the previous release channel branch name (``v{major}.{minor-1}``).

    Used by upmerge workflows which run from the *previous* branch.
    Example: version='0.56.0-rc1' -> 'v0.55'.
    """
    parts = version.split(".")
    major, minor = int(parts[0]), int(parts[1])
    if minor == 0:
        error("Cannot compute previous channel for minor=0")
    return f"v{major}.{minor - 1}"


def gh_commit_files(
    repo: str, branch: str, message: str, files: dict[str, str]
) -> None:
    """Commit one or more files to a branch via the Git Data API (atomic).

    files: mapping of {repo_path: file_content}
    """
    # Get current branch tip
    parent_sha = gh_get_branch_sha(repo, branch)
    commit_data = gh_api_json(f"repos/{repo}/git/commits/{parent_sha}")
    base_tree_sha = commit_data["tree"]["sha"]

    # Create blobs for each file
    tree_entries: list[dict[str, str]] = []
    for file_path, content in files.items():
        blob = gh_api_json(
            f"repos/{repo}/git/blobs",
            method="POST",
            body={"content": content, "encoding": "utf-8"},
        )
        tree_entries.append(
            {
                "path": file_path,
                "mode": "100644",
                "type": "blob",
                "sha": blob["sha"],
            }
        )

    # Create tree
    new_tree = gh_api_json(
        f"repos/{repo}/git/trees",
        method="POST",
        body={"base_tree": base_tree_sha, "tree": tree_entries},
    )

    # Append Signed-off-by trailer
    if cfg.user_name and cfg.user_email:
        message += f"\n\nSigned-off-by: {cfg.user_name} <{cfg.user_email}>"

    # Create commit
    new_commit = gh_api_json(
        f"repos/{repo}/git/commits",
        method="POST",
        body={
            "message": message,
            "tree": new_tree["sha"],
            "parents": [parent_sha],
        },
    )

    # Update branch ref
    gh_api_json(
        f"repos/{repo}/git/refs/heads/{branch}",
        method="PATCH",
        body={"sha": new_commit["sha"]},
    )
    short_sha = new_commit["sha"][:7]
    log(
        f"Committed {short_sha} to {branch}: https://github.com/{repo}/commit/{new_commit['sha']}"
    )


def gh_compare_branches(repo: str, base: str, head: str) -> dict:
    """Compare two refs via the GitHub compare API.

    Returns the raw compare response containing ``commits`` (up to 250)
    and ``files`` (up to 300).  Warns if the response appears truncated.
    """
    data = gh_api_json(f"repos/{repo}/compare/{base}...{head}")
    total = data.get("total_commits", 0)
    commits = data.get("commits", [])
    files = data.get("files", [])
    if total > len(commits):
        warn(
            f"Compare response truncated: showing {len(commits)}/{total} commits. "
            "Consider using a local cherry-pick for very large ranges."
        )
    if len(files) >= 300:
        warn(
            "Compare response may have truncated the file list (300 file limit). "
            "Consider using a local cherry-pick for very large ranges."
        )
    return data


def gh_get_commit_detail(repo: str, sha: str) -> dict:
    """Fetch a single commit's metadata and changed files."""
    return gh_api_json(f"repos/{repo}/commits/{sha}")


def _build_tree_entries_for_files(
    compare_files: list[dict[str, Any]],
) -> list[dict[str, Any]]:
    """Convert a list of file-change dicts into Git tree entries.

    Reuses the blob SHA already present in each file dict so no extra
    blob uploads are needed.
    """
    entries: list[dict[str, Any]] = []
    for f in compare_files:
        path = f["filename"]
        status = f.get("status", "")
        blob_sha = f.get("sha")

        if status in ("added", "modified", "changed", "copied"):
            entries.append(
                {"path": path, "mode": "100644", "type": "blob", "sha": blob_sha}
            )
        elif status == "removed":
            entries.append(
                {"path": path, "mode": "100644", "type": "blob", "sha": None}
            )
        elif status == "renamed":
            prev = f.get("previous_filename", "")
            if prev:
                entries.append(
                    {"path": prev, "mode": "100644", "type": "blob", "sha": None}
                )
            entries.append(
                {"path": path, "mode": "100644", "type": "blob", "sha": blob_sha}
            )
    return entries


def gh_cherrypick_commit(
    repo: str,
    target_branch: str,
    original_commit: dict,
) -> str:
    """Cherry-pick a single commit onto *target_branch* via the Git Data API.

    Preserves the original commit message with a ``(cherry picked from commit …)``
    trailer, the original author, and a Signed-off-by line.

    Returns the SHA of the newly created commit.
    """
    original_sha = original_commit["sha"]
    commit_meta = original_commit.get("commit", {})
    original_message = commit_meta.get("message", "")
    original_author = commit_meta.get("author", {})

    # Fetch per-commit file changes
    detail = gh_get_commit_detail(repo, original_sha)
    files = detail.get("files", [])
    if not files:
        debug(f"Commit {original_sha[:7]} has no file changes – skipping")
        return gh_get_branch_sha(repo, target_branch)

    # Build tree entries from the commit's own file list
    tree_entries = _build_tree_entries_for_files(files)
    if not tree_entries:
        debug(f"Commit {original_sha[:7]} produced no tree entries – skipping")
        return gh_get_branch_sha(repo, target_branch)

    # Current tip of the target branch
    parent_sha = gh_get_branch_sha(repo, target_branch)
    parent_commit = gh_api_json(f"repos/{repo}/git/commits/{parent_sha}")
    base_tree_sha = parent_commit["tree"]["sha"]

    # Create tree (may fail 404 if commits touch .github/workflows/ and
    # the token lacks the 'workflow' scope).
    new_tree = gh_api_json(
        f"repos/{repo}/git/trees",
        method="POST",
        body={"base_tree": base_tree_sha, "tree": tree_entries},
        silent_fail=True,
    )
    if new_tree is None:
        workflow_files = [
            e["path"]
            for e in tree_entries
            if e["path"].startswith(".github/workflows/")
        ]
        if workflow_files:
            error(
                f"Tree creation failed for commit {original_sha[:7]}.\n"
                "  This is most likely because the commit modifies .github/workflows/ files\n"
                "  and the gh token is missing the 'workflow' scope.\n\n"
                "  Fix:\n"
                "    gh auth refresh -s workflow\n\n"
                f"  Affected workflow file(s): {', '.join(workflow_files)}"
            )
        error(
            f"Tree creation failed for commit {original_sha[:7]}.\n"
            "  Run with DEBUG logging and verify gh token scopes."
        )

    # Build commit message
    message = original_message.rstrip()
    message += f"\n\n(cherry picked from commit {original_sha})"
    if cfg.user_name and cfg.user_email:
        message += f"\nSigned-off-by: {cfg.user_name} <{cfg.user_email}>"

    # Preserve original author
    author_payload: dict[str, str] = {}
    if original_author.get("name") and original_author.get("email"):
        author_payload = {
            "name": original_author["name"],
            "email": original_author["email"],
            "date": original_author.get("date", ""),
        }

    commit_body: dict[str, Any] = {
        "message": message,
        "tree": new_tree["sha"],
        "parents": [parent_sha],
    }
    if author_payload:
        commit_body["author"] = author_payload

    new_commit = gh_api_json(
        f"repos/{repo}/git/commits",
        method="POST",
        body=commit_body,
    )

    # Fast-forward the branch ref
    gh_api_json(
        f"repos/{repo}/git/refs/heads/{target_branch}",
        method="PATCH",
        body={"sha": new_commit["sha"]},
    )

    short_old = original_sha[:7]
    short_new = new_commit["sha"][:7]
    first_line = original_message.split("\n")[0]
    log(f"  cherry-picked {short_old} -> {short_new}  {first_line}")
    return new_commit["sha"]


def gh_cherrypick_commits(
    repo: str,
    target_branch: str,
    commits: list[dict],
    extra_files: dict[str, str] | None = None,
) -> None:
    """Cherry-pick a sequence of commits onto *target_branch*.

    Each commit is applied individually, preserving the original history.
    After all commits are replayed, any *extra_files* are committed on
    top as a separate housekeeping commit.
    """
    for commit in commits:
        gh_cherrypick_commit(repo, target_branch, commit)

    # Extra files (e.g. release notes) as a final commit
    if extra_files:
        gh_commit_files(
            repo,
            target_branch,
            f"chore(release): additional release files for v{cfg.version}",
            extra_files,
        )


# =============================================================================
# Local git helpers (used for patch mode cherry-pick)
# =============================================================================


def validate_repo(repo_path: str, expected_remote: str) -> None:
    p = Path(repo_path)
    if not p.is_dir():
        error(f"Repository path not found: {repo_path}")
    if not (p / ".git").is_dir():
        error(f"Not a git repository: {repo_path}")

    result = run_cmd(["git", "-C", repo_path, "remote", "get-url", "origin"])
    remote = result.stdout.strip()
    if expected_remote not in remote:
        error(
            f"Repo {repo_path} origin is '{remote}', expected to contain '{expected_remote}'"
        )


def validate_clean_worktree(repo_path: str) -> None:
    result = run_cmd(["git", "-C", repo_path, "status", "--porcelain"])
    if result.stdout.strip():
        error(f"Repository has uncommitted changes: {repo_path}")


def git_checkout_and_pull_base(repo_path: str, base: str) -> None:
    run_readonly(["git", "-C", repo_path, "fetch", "origin"])
    run_mutating(
        f"Checkout {base} in {repo_path}", ["git", "-C", repo_path, "checkout", base]
    )
    run_mutating(
        f"Pull latest {base} in {repo_path}",
        ["git", "-C", repo_path, "pull", "origin", base],
    )


def create_or_reset_work_branch(
    repo_path: str, from_branch: str, work_branch: str
) -> None:
    # Check local branch
    result = run_cmd(
        [
            "git",
            "-C",
            repo_path,
            "show-ref",
            "--verify",
            "--quiet",
            f"refs/heads/{work_branch}",
        ],
        check=False,
    )
    if result.returncode == 0:
        error(f"Local work branch already exists: {work_branch}")

    # Check remote branch
    result = run_cmd(
        [
            "git",
            "-C",
            repo_path,
            "ls-remote",
            "--exit-code",
            "--heads",
            "origin",
            work_branch,
        ],
        check=False,
    )
    if result.returncode == 0:
        error(f"Remote work branch already exists: {work_branch}")

    run_mutating(
        f"Create work branch {work_branch} from {from_branch}",
        ["git", "-C", repo_path, "checkout", "-b", work_branch, from_branch],
    )


# =============================================================================
# Prerequisites and branch preparation
# =============================================================================


def ensure_gh_auth() -> None:
    """Verify gh CLI is authenticated."""
    result = run_cmd(["gh", "auth", "status"], check=False)
    if result.returncode != 0:
        error("gh CLI is not authenticated. Run 'gh auth login' first.")


def gh_token_scopes() -> set[str]:
    """Return the set of OAuth scopes on the current ``gh`` token."""
    result = run_cmd(["gh", "auth", "status"], check=False)
    output = (result.stdout or "") + (result.stderr or "")
    for line in output.splitlines():
        if "Token scopes" in line:
            # Line format: "  - Token scopes: 'repo', 'workflow'"
            raw = line.split(":", 1)[-1].strip()
            return {s.strip().strip("'\"") for s in raw.split(",") if s.strip()}
    return set()


def ensure_workflow_scope() -> None:
    """Error if the gh token lacks the ``workflow`` scope.

    The ``workflow`` scope is required to create/modify files under
    ``.github/workflows/`` via the Git Data API **and** ``git push``.
    Without it, any cherry-pick that includes workflow file changes
    will fail with HTTP 404.
    """
    scopes = gh_token_scopes()
    if "workflow" not in scopes:
        error(
            "gh token is missing the 'workflow' scope.\n"
            "  Cherry-pick may include .github/workflows/ files which require\n"
            "  this scope for both the REST API and git push.\n\n"
            "  Fix:\n"
            "    gh auth refresh -s workflow\n\n"
            f"  Current scopes: {', '.join(sorted(scopes)) or '(none)'}"
        )


def ensure_prerequisites() -> None:
    """Validate repos and access for the current mode."""
    gh_verify_repo(RADIUS_GH_REPO)

    match cfg.mode:
        case "rc" | "final":
            gh_verify_repo(DE_GH_REPO)
        case "patch":
            # --radius-repo is only needed for Phase 2 (local cherry-pick).
            # Phase 1 (versions.yaml PR) uses the GitHub API exclusively.
            if cfg.radius_repo:
                validate_repo(cfg.radius_repo, "radius-project/radius")
                validate_clean_worktree(cfg.radius_repo)


def prepare_branch_names() -> None:
    channel = channel_from_version(cfg.version)
    if not cfg.release_branch:
        cfg.release_branch = f"release/{channel}"
    if not cfg.work_branch:
        cfg.work_branch = f"{cfg.username}/release-{cfg.version}"


# =============================================================================
# Auto-version resolution
# Fetches versions.yaml from GitHub and calculates the next version.
# =============================================================================


def auto_resolve_version() -> None:
    log(f"Auto-calculating version for mode '{cfg.mode}' from versions.yaml...")

    content = gh_get_file_content(RADIUS_GH_REPO, "versions.yaml", cfg.base_branch)
    data = parse_versions_yaml(content)

    if not data["supported"]:
        error("Cannot auto-calculate version: versions.yaml has no supported entries")

    newest = data["supported"][0]
    newest_version = newest.get("version", "")
    newest_channel = newest.get("channel", "")

    if not newest_version or not newest_channel:
        error("Cannot auto-calculate version: versions.yaml has no supported entries")

    ver = newest_version.lstrip("v")
    log(f"Newest supported entry: channel={newest_channel}, version={newest_version}")

    parts = newest_channel.split(".")
    major, minor = int(parts[0]), int(parts[1])

    match cfg.mode:
        case "rc":
            m = re.search(r"-rc(\d+)$", ver)
            if m:
                rc_num = int(m.group(1))
                base_ver = ver.split("-rc")[0]
                cfg.version = f"{base_ver}-rc{rc_num + 1}"
                log(f"Existing RC detected (rc{rc_num}). Next version: {cfg.version}")
            else:
                cfg.version = f"{major}.{minor + 1}.0-rc1"
                log(f"No active RC. Bumping minor version. Next version: {cfg.version}")
        case "final":
            if "-rc" in ver:
                cfg.version = ver.split("-rc")[0]
                log(f"Promoting RC to final. Next version: {cfg.version}")
            else:
                cfg.version = f"{major}.{minor + 1}.0"
                log(f"No active RC. Bumping minor version. Next version: {cfg.version}")
        case _:
            error(f"Auto-version is not supported for mode '{cfg.mode}'")

    log(f"Auto-resolved version: v{cfg.version}")


# =============================================================================
# versions.yaml update (API-based)
# Fetches from GitHub, validates, modifies in memory, returns new content.
# =============================================================================


def gh_prepare_versions_yaml(ref: str | None = None) -> str:
    """Fetch, validate, modify versions.yaml and return new content.

    Args:
        ref: Git ref to fetch versions.yaml from.  Defaults to cfg.base_branch.
    """
    source_ref = ref or cfg.base_branch
    channel = channel_from_version(cfg.version)
    target = f"v{cfg.version}"

    log(f"Fetching versions.yaml from {RADIUS_GH_REPO} (ref: {source_ref})")
    content = gh_get_file_content(RADIUS_GH_REPO, "versions.yaml", source_ref)
    data = parse_versions_yaml(content)

    # Find existing entries for this channel
    matching = [e for e in data["supported"] if e["channel"] == channel]
    if len(matching) == 1 and matching[0]["version"] == target:
        error(f"versions.yaml already contains supported channel {channel} -> {target}")
    if len(matching) > 1:
        error(f"versions.yaml has duplicate supported entries for channel {channel}")

    if len(matching) == 1:
        log(f"Updating existing supported version for channel {channel} to {target}")
        for entry in data["supported"]:
            if entry["channel"] == channel:
                entry["version"] = target
                break
    else:
        log(f"Inserting new supported channel {channel} with {target}")
        data["supported"].insert(0, {"channel": channel, "version": target})

    # (Fix #9) Auto-manage deprecated: demote oldest supported when >2 channels
    MAX_SUPPORTED_CHANNELS = 2
    while len(data["supported"]) > MAX_SUPPORTED_CHANNELS:
        demoted = data["supported"].pop()
        log(
            f"Demoting channel {demoted['channel']} ({demoted['version']}) to deprecated"
        )
        data["deprecated"].insert(0, demoted)

    # Validate the modification
    updated = [e for e in data["supported"] if e["channel"] == channel]
    if not updated or updated[0]["version"] != target:
        error(
            f"versions.yaml modification validation failed for channel {channel}. "
            f"Got '{updated[0]['version'] if updated else ''}', expected '{target}'"
        )

    return emit_versions_yaml(data)


# =============================================================================
# Deployment engine tagging (API-based, no local clone required)
# =============================================================================


def tag_and_push_deployment_engine() -> None:
    """Create an annotated tag in deployment-engine via the Git Data API.

    Uses two API calls:
      1. POST /repos/{repo}/git/tags   – create the tag object
      2. POST /repos/{repo}/git/refs   – create the ref pointing to it

    No local clone or GPG key is required.
    """
    tag_name = f"v{cfg.version}"

    gh_assert_tag_not_exists(DE_GH_REPO, tag_name)

    # Resolve the SHA of the main branch tip
    main_sha = gh_get_branch_sha(DE_GH_REPO, "main")
    log(f"DE main branch SHA: {main_sha[:7]}")

    # Create the annotated tag object
    run_mutating_api(
        f"Create annotated tag {tag_name} in {DE_GH_REPO}",
        gh_api_json,
        f"repos/{DE_GH_REPO}/git/tags",
        method="POST",
        body={
            "tag": tag_name,
            "message": tag_name,
            "object": main_sha,
            "type": "commit",
        },
    )

    # Create the ref pointing to the tag
    run_mutating_api(
        f"Create tag ref {tag_name} in {DE_GH_REPO}",
        gh_api_json,
        f"repos/{DE_GH_REPO}/git/refs",
        method="POST",
        body={
            "ref": f"refs/tags/{tag_name}",
            "sha": main_sha,
        },
    )

    log(f"DE tag: https://github.com/{DE_GH_REPO}/releases/tag/{tag_name}")


# =============================================================================
# PR creation (gh CLI based, parameterised)
# =============================================================================


def create_release_pr(repo: str, base: str, head: str, title: str, body: str) -> None:
    # Check for existing open PR
    result = run_cmd(
        [
            "gh",
            "pr",
            "list",
            "--repo",
            repo,
            "--base",
            base,
            "--head",
            head,
            "--state",
            "open",
            "--json",
            "number",
            "--jq",
            "length",
        ]
    )
    if result.stdout.strip() != "0":
        error(f"An open PR already exists for {head} -> {base} in {repo}")

    if not cfg.execute_mutations:
        log(f"DRY-RUN (mutation blocked): Create PR '{title}' ({head} -> {base})")
        log(f"  Compare: https://github.com/{repo}/compare/{base}...{head}")
        return

    confirm_or_exit(f"Proceed with: Create PR {head} -> {base}?")
    result = run_cmd(
        [
            "gh",
            "pr",
            "create",
            "--repo",
            repo,
            "--base",
            base,
            "--head",
            head,
            "--title",
            title,
            "--body",
            body,
        ]
    )
    url = result.stdout.strip()
    log(f"Created PR: {url}")

    if cfg.auto_merge_pr:
        confirm_or_exit(f"Queue auto-merge for PR {url}?")
        run_cmd(["gh", "pr", "merge", "--repo", repo, "--squash", "--auto", url])
        log(f"Auto-merge queued for {url}")


# =============================================================================
# Workflow dispatch (gh CLI based)
# =============================================================================


def _dispatch_workflow_and_log_run(
    *,
    description: str,
    repo: str,
    workflow_file: str,
    ref: str,
    fields: list[str] | None = None,
) -> None:
    """Dispatch a workflow and log the direct run URL when available."""
    cmd = [
        "gh",
        "workflow",
        "run",
        workflow_file,
        "--repo",
        repo,
        "--ref",
        ref,
    ]
    if fields:
        for field in fields:
            cmd.extend(["-f", field])

    result = run_mutating(description, cmd)
    if result is None:
        log("  Run: (dry-run) no run was triggered")
        return

    run_url = wait_for_dispatched_run_url(repo, workflow_file, ref)
    if run_url:
        log(f"  Run: {run_url}")
    else:
        warn(
            f"Could not resolve run URL for {repo}/{workflow_file} on ref {ref} yet. "
            "The run may still be initializing."
        )


def dispatch_radius_release_verification() -> None:
    gh_assert_branch_exists(RADIUS_GH_REPO, cfg.release_branch)

    _dispatch_workflow_and_log_run(
        description=f"Dispatch release-verification workflow for {cfg.version} on {cfg.release_branch}",
        repo=RADIUS_GH_REPO,
        workflow_file="release-verification.yaml",
        ref=cfg.release_branch,
        fields=[f"version={cfg.version}"],
    )


def _dispatch_cross_repo_workflow(
    repo: str,
    workflow: str,
    ref: str,
    label: str,
    *,
    fields: list[str] | None = None,
) -> None:
    """Thin wrapper around ``_dispatch_workflow_and_log_run`` with standard description."""
    _dispatch_workflow_and_log_run(
        description=f"Dispatch {label} for v{cfg.version} on {ref}",
        repo=repo,
        workflow_file=workflow,
        ref=ref,
        fields=fields,
    )


def dispatch_docs_upmerge() -> None:
    ref = previous_channel_branch(cfg.version)
    _dispatch_cross_repo_workflow(DOCS_GH_REPO, "upmerge.yaml", ref, "docs upmerge")


def dispatch_samples_upmerge() -> None:
    ref = previous_channel_branch(cfg.version)
    _dispatch_cross_repo_workflow(
        SAMPLES_GH_REPO, "upmerge.yaml", ref, "samples upmerge"
    )


def dispatch_docs_release() -> None:
    _dispatch_cross_repo_workflow(
        DOCS_GH_REPO,
        "release.yaml",
        "edge",
        "docs release",
        fields=[f"version={cfg.version}"],
    )


def dispatch_samples_release() -> None:
    _dispatch_cross_repo_workflow(
        SAMPLES_GH_REPO,
        "release.yaml",
        "edge",
        "samples release",
        fields=[f"version={cfg.version}"],
    )


def dispatch_samples_test() -> None:
    _dispatch_cross_repo_workflow(
        SAMPLES_GH_REPO,
        "test.yaml",
        "edge",
        "samples test",
        fields=[f"version={cfg.version}"],
    )


# ---------------------------------------------------------------------------
# Post-merge automation
# ---------------------------------------------------------------------------


def _check_build_workflow(tag: str) -> str | None:
    """Check the status of the build workflow triggered by *tag*."""
    run_info = gh_get_workflow_run_for_tag(RADIUS_GH_REPO, "build.yaml", tag)
    if run_info is None:
        warn(
            f"No build.yaml workflow run found for tag {tag} - it may not have started yet."
        )
        return None
    else:
        status = run_info.get("status", "unknown")
        conclusion = run_info.get("conclusion", "")
        url = run_info.get("url", "")
        if conclusion == "success":
            log(f"Build workflow for {tag} completed successfully: {url}")
        elif status in ("completed",) and conclusion != "success":
            warn(
                f"Build workflow for {tag} finished with conclusion '{conclusion}': {url}"
            )
        else:
            log(
                f"Build workflow for {tag} is {status} (conclusion: {conclusion or 'n/a'}): {url}"
            )
        return url or None


def run_post_merge_steps() -> None:
    """Run post-merge workflow dispatches.

    Gate: The GitHub Release for this version must exist (meaning the
    build workflow completed and published the release artifacts).
    Dispatches the appropriate cross-repo workflows based on mode.
    """
    tag = f"v{cfg.version}"

    # Gate check: tag must exist in radius repo
    if not gh_tag_exists(RADIUS_GH_REPO, tag):
        warn(
            f"Post-merge: Tag {tag} does not exist in {RADIUS_GH_REPO} yet "
            "- skipping post-merge steps."
        )
        return

    # Check build workflow status
    build_run_url = _check_build_workflow(tag)

    # Gate check: release must exist (build workflow finished successfully)
    if not gh_release_exists(RADIUS_GH_REPO, tag):
        log(
            f"Post-merge: GitHub Release for {tag} does not exist yet. "
            "The build workflow may still be running."
        )
        if build_run_url:
            log(f"  Build run: {build_run_url}")
        log(f"Re-run with:\n  {_build_rerun_command()}")
        return

    log(
        f"Post-merge: GitHub Release for {tag} exists "
        "- proceeding with workflow dispatches."
    )
    log(f"  Release: https://github.com/{RADIUS_GH_REPO}/releases/tag/{tag}")

    # (Fix #10) Prominent bicep-types-aws reminder
    log("=" * 60)
    log("ACTION REQUIRED: bicep-types-aws repository")
    log(
        f"  The tag {tag} should have triggered the 'Update extensibility\n"
        "  provider types' workflow in the bicep-types-aws repository.\n"
        "  Please verify and approve the workflow run at:\n"
        "  https://github.com/radius-project/bicep-types-aws/actions"
    )
    log("=" * 60)

    # Mode-specific dispatches
    # (Fix #8) For final mode, dispatch order matches README:
    #   docs release -> samples release -> release verification -> samples test
    # (Fix #5) For RC mode, do NOT auto-dispatch samples test because
    #   upmerge PRs must be merged to edge first.
    match cfg.mode:
        case "rc":
            dispatch_docs_upmerge()
            dispatch_samples_upmerge()
            if cfg.run_radius_verification:
                dispatch_radius_release_verification()
            log(
                "ACTION REQUIRED: Wait for upmerge PRs to be reviewed, "
                "approved, and merged to edge before running samples test."
            )
            log(
                "  Once upmerge PRs are merged, run samples test manually:\n"
                f"  gh workflow run test.yaml --repo {SAMPLES_GH_REPO} "
                f"--ref edge -f version={cfg.version}"
            )

        case "final":
            dispatch_docs_release()
            dispatch_samples_release()
            if cfg.run_radius_verification:
                dispatch_radius_release_verification()
            dispatch_samples_test()

        case "patch":
            if cfg.run_radius_verification:
                dispatch_radius_release_verification()
            dispatch_samples_test()

    log("Post-merge workflow dispatches completed.")


# =============================================================================
# Cherry-pick to release branch (API-based)
# =============================================================================


def _cherrypick_to_release_branch() -> None:
    """Cherry-pick the merged release PR commit onto the release branch.

    Only cherry-picks the specific merge commit from the work-branch PR
    (not all ahead commits). This ensures the release branch receives
    exactly the intended changes (versions.yaml, release notes, etc.)
    without pulling in unrelated commits.

    For the first RC of a new channel, creates the release branch from
    the base branch (which already contains the merged commit), so no
    cherry-pick is needed.

    Idempotent / resume-aware: skips when the release branch already has
    the target version, or when the cherry-pick PR already exists.
    """
    # (Fix #6) Auto-create release branch for first RC
    if not gh_branch_exists(RADIUS_GH_REPO, cfg.release_branch):
        if cfg.mode == "rc":
            log(
                f"Release branch {cfg.release_branch} does not exist "
                f"- creating from {cfg.base_branch}"
            )
            run_mutating_api(
                f"Create release branch {cfg.release_branch} from {cfg.base_branch}",
                gh_create_branch_from_ref,
                RADIUS_GH_REPO,
                cfg.release_branch,
                cfg.base_branch,
            )
            log(
                f"Release branch {cfg.release_branch} created from "
                f"{cfg.base_branch}. No cherry-pick needed (branch already "
                "includes the merged commit)."
            )
            return
        else:
            error(
                f"Release branch {cfg.release_branch} does not exist. "
                "It should have been created during the RC process."
            )

    # Ensure the token can push workflow file changes
    ensure_workflow_scope()

    # Already up to date?
    if gh_versions_yaml_has_version(RADIUS_GH_REPO, cfg.release_branch, cfg.version):
        warn(
            f"SKIP cherry-pick: versions.yaml on {cfg.release_branch} already "
            f"contains v{cfg.version}"
        )
        return

    cherrypick_branch = f"{cfg.username}/cherrypick-{cfg.version}"

    # Check for existing cherry-pick PR
    pr_info = gh_find_pr(RADIUS_GH_REPO, cfg.release_branch, cherrypick_branch)
    if pr_info:
        state_result = handle_existing_pr_state(
            step_label="cherry-pick",
            pr_info=pr_info,
            merged_message=(
                f"SKIP cherry-pick: PR already merged: {pr_info.get('url', '')}"
            ),
            open_message=(
                f"SKIP cherry-pick: PR already open: {pr_info.get('url', '')}"
            ),
            other_message=(
                f"Cherry-pick PR exists in state "
                f"'{pr_info.get('state', '').upper()}': {pr_info.get('url', '')}"
            ),
        )
        if state_result in ("merged", "open"):
            return

    # (Fix #1, #3) Find the specific merged PR commit to cherry-pick
    merged_pr = gh_find_pr(RADIUS_GH_REPO, cfg.base_branch, cfg.work_branch)
    if not merged_pr:
        error(
            f"Cannot find PR for {cfg.work_branch} -> {cfg.base_branch}. "
            "Ensure the release PR has been created and merged before re-running."
        )
    if merged_pr.get("state") != "MERGED":
        error(
            f"Release PR {merged_pr.get('url', '')} is not merged "
            f"(state: {merged_pr.get('state', '')}). "
            "Merge the PR first, then re-run."
        )

    merge_sha = gh_get_pr_merge_commit_sha(RADIUS_GH_REPO, merged_pr["number"])
    if not merge_sha:
        error(f"Could not determine merge commit SHA for PR #{merged_pr['number']}")

    log(f"Found merged PR #{merged_pr['number']}: {merged_pr.get('url', '')}")
    log(f"  Merge commit: {merge_sha[:7]}")

    # Fetch the merge commit details
    merge_commit_detail = gh_get_commit_detail(RADIUS_GH_REPO, merge_sha)
    first_line = merge_commit_detail.get("commit", {}).get("message", "").split("\n")[0]
    files = merge_commit_detail.get("files", [])

    log(f"  Message: {first_line}")
    log(f"  Files changed: {len(files)}")
    for f in files:
        log(f"    {f.get('status', '?')} {f['filename']}")

    # Build PR body
    pr_body_lines = [
        f"Cherry-pick merge commit `{merge_sha[:7]}` from "
        f"PR #{merged_pr['number']} (`{cfg.work_branch}` -> "
        f"`{cfg.base_branch}`) into `{cfg.release_branch}` "
        f"for **v{cfg.version}**.",
        "",
        f"Source PR: {merged_pr.get('url', '')}",
        "",
        "| File | Status |",
        "|------|--------|",
    ]
    for f in files:
        pr_body_lines.append(f"| `{f['filename']}` | {f.get('status', '?')} |")

    # Create cherry-pick branch and replay the commit
    commits_needed = True
    if gh_branch_exists(RADIUS_GH_REPO, cherrypick_branch):
        warn(f"Cherry-pick branch {cherrypick_branch} already exists")
        if gh_versions_yaml_has_version(RADIUS_GH_REPO, cherrypick_branch, cfg.version):
            warn(
                f"SKIP commits: versions.yaml on {cherrypick_branch} already up to date"
            )
            commits_needed = False
    else:
        run_mutating_api(
            f"Create branch {cherrypick_branch} from {cfg.release_branch}",
            gh_create_branch_from_ref,
            RADIUS_GH_REPO,
            cherrypick_branch,
            cfg.release_branch,
        )

    if commits_needed:
        run_mutating_api(
            f"Cherry-pick merge commit {merge_sha[:7]} onto {cherrypick_branch}",
            gh_cherrypick_commit,
            RADIUS_GH_REPO,
            cherrypick_branch,
            merge_commit_detail,
        )

    # Create PR
    create_release_pr(
        RADIUS_GH_REPO,
        cfg.release_branch,
        cherrypick_branch,
        f"chore(release): cherry-pick v{cfg.version} to {cfg.release_branch}",
        "\n".join(pr_body_lines),
    )


# =============================================================================
# Release flows
# =============================================================================


def _create_versioning_pr(
    *,
    extra_files: dict[str, str] | None = None,
    pr_title: str,
    pr_body: str,
) -> None:
    """Steps 2-5: prepare versions.yaml, create work branch, commit, and create PR.

    Resume-aware: skips already-completed steps.

    Args:
        extra_files: Additional files to commit alongside versions.yaml.
        pr_title: Title for the release PR.
        pr_body: Body text for the release PR.
    """
    # Step 2: Prepare versions.yaml content
    log("Step 2: Preparing versions.yaml update")
    versions_content = gh_prepare_versions_yaml()

    files: dict[str, str] = {"versions.yaml": versions_content}
    if extra_files:
        files.update(extra_files)

    # Step 3: Create work branch (if needed)
    commit_needed = True
    if gh_branch_exists(RADIUS_GH_REPO, cfg.work_branch):
        warn(f"SKIP step 3: Work branch {cfg.work_branch} already exists")
        log(f"  Branch: https://github.com/{RADIUS_GH_REPO}/tree/{cfg.work_branch}")
        if gh_versions_yaml_has_version(RADIUS_GH_REPO, cfg.work_branch, cfg.version):
            warn(f"SKIP step 4: Files already committed to {cfg.work_branch}")
            commit_needed = False
    else:
        log(f"Step 3: Creating work branch {cfg.work_branch}")
        run_mutating_api(
            f"Create branch {cfg.work_branch} from {cfg.base_branch} in {RADIUS_GH_REPO}",
            gh_create_branch_from_ref,
            RADIUS_GH_REPO,
            cfg.work_branch,
            cfg.base_branch,
        )

    # Step 4: Commit files
    if commit_needed:
        file_desc = ", ".join(files.keys())
        log(f"Step 4: Committing {file_desc} to {cfg.work_branch}")
        run_mutating_api(
            f"Commit {file_desc} to {cfg.work_branch} via GitHub API",
            gh_commit_files,
            RADIUS_GH_REPO,
            cfg.work_branch,
            f"chore(release): v{cfg.version}",
            files,
        )

    # Step 5: Create PR
    pr_info = gh_find_pr(RADIUS_GH_REPO, cfg.base_branch, cfg.work_branch)
    if pr_info:
        handle_existing_pr_state(step_label="step 5", pr_info=pr_info)
    else:
        log("Step 5: Creating release PR")
        create_release_pr(
            RADIUS_GH_REPO,
            cfg.base_branch,
            cfg.work_branch,
            pr_title,
            pr_body,
        )


def run_rc_flow() -> None:
    tag = f"v{cfg.version}"
    log(f"Starting RC release flow for v{cfg.version} (resume-aware)")

    # Step 1: Tag deployment-engine
    if gh_tag_exists(DE_GH_REPO, tag):
        warn(f"SKIP step 1: Tag {tag} already exists in {DE_GH_REPO}")
        log(f"  Tag: https://github.com/{DE_GH_REPO}/releases/tag/{tag}")
    else:
        log(f"Step 1: Tagging deployment-engine with {tag}")
        tag_and_push_deployment_engine()

    # Early exit: if versions.yaml on the base branch already has the version
    # then the PR was merged and all steps are complete.
    if gh_versions_yaml_has_version(RADIUS_GH_REPO, cfg.base_branch, cfg.version):
        warn(
            f"SKIP steps 2-5: versions.yaml on {cfg.base_branch} already "
            f"contains v{cfg.version} (PR merged)"
        )
        # For subsequent RCs (rc2+): cherry-pick versions.yaml to release branch
        _cherrypick_to_release_branch()
        run_post_merge_steps()
        log("RC flow completed.")
        return

    # Steps 2-5: prepare, branch, commit, PR
    _create_versioning_pr(
        pr_title=f"chore(release): v{cfg.version}",
        pr_body=f"Automated RC release preparation for v{cfg.version}.",
    )

    if cfg.run_radius_verification:
        dispatch_radius_release_verification()

    # Hint about cherry-pick for subsequent RCs
    if gh_branch_exists(RADIUS_GH_REPO, cfg.release_branch):
        log(
            f"NOTE: After the main PR is merged, re-run the script to "
            f"cherry-pick versions.yaml into {cfg.release_branch}."
        )

    log("RC flow completed. Merge the PR, then re-run to trigger post-merge workflows:")
    log(f"  {_build_rerun_command()}")


def run_final_flow() -> None:
    tag = f"v{cfg.version}"
    log(f"Starting final release flow for v{cfg.version} (resume-aware)")

    if not cfg.release_notes_file:
        error("--release-notes-file is required for final mode")
    notes_path = Path(cfg.release_notes_file)
    if not notes_path.is_file():
        error(f"Release notes file not found: {cfg.release_notes_file}")

    # Step 1: Tag deployment-engine
    if gh_tag_exists(DE_GH_REPO, tag):
        warn(f"SKIP step 1: Tag {tag} already exists in {DE_GH_REPO}")
        log(f"  Tag: https://github.com/{DE_GH_REPO}/releases/tag/{tag}")
    else:
        log(f"Step 1: Tagging deployment-engine with {tag}")
        tag_and_push_deployment_engine()

    # Read release notes from local file and resolve repo commit path
    release_notes_content = notes_path.read_text(encoding="utf-8")
    release_notes_repo_path = cfg.resolved_release_notes_repo_path
    if not release_notes_repo_path:
        error("Internal error: release notes repo path was not resolved")
    log(f"Release notes repo path: {release_notes_repo_path}")

    # Early exit: if versions.yaml on base branch already has the version
    if gh_versions_yaml_has_version(RADIUS_GH_REPO, cfg.base_branch, cfg.version):
        warn(
            f"SKIP steps 2-5: versions.yaml on {cfg.base_branch} already "
            f"contains v{cfg.version} (PR merged)"
        )
        # Cherry-pick the merged commit (containing versions.yaml + release
        # notes) to the release branch. No extra_files needed because the
        # single merged commit already contains both files.
        _cherrypick_to_release_branch()
        run_post_merge_steps()
        log("Final flow completed.")
        return

    # Steps 2-5: prepare, branch, commit, PR
    _create_versioning_pr(
        extra_files={release_notes_repo_path: release_notes_content},
        pr_title=f"Release v{cfg.version}",
        pr_body=f"Automated final release preparation for v{cfg.version}.",
    )

    log(
        f"After the main PR is merged, re-run the script to cherry-pick "
        f"release changes into {cfg.release_branch}."
    )
    if cfg.run_radius_verification:
        dispatch_radius_release_verification()

    log(
        "Final flow completed. Merge the PR, then re-run to trigger post-merge workflows:"
    )
    log(f"  {_build_rerun_command()}")


def run_patch_flow() -> None:
    log(f"Starting patch release flow for v{cfg.version} (resume-aware)")

    if not cfg.bugfix_commit:
        error("--bugfix-commit is required for patch mode")

    # Verify bugfix commit exists via API
    gh_require_commit(RADIUS_GH_REPO, cfg.bugfix_commit)

    # Verify release branch exists via API
    gh_assert_branch_exists(RADIUS_GH_REPO, cfg.release_branch)

    # Phase 1: Ensure versions.yaml is updated on main (Fix #7)
    # If --version-commit is not provided, automate the versions.yaml PR.
    version_commit = cfg.version_commit
    if not version_commit:
        version_work_branch = f"{cfg.username}/patch-versions-{cfg.version}"

        if gh_versions_yaml_has_version(RADIUS_GH_REPO, cfg.base_branch, cfg.version):
            log(
                f"versions.yaml on {cfg.base_branch} already contains "
                f"v{cfg.version} - auto-detecting version commit"
            )
            # Try to find the merged PR to get the version commit
            version_pr = gh_find_pr(
                RADIUS_GH_REPO, cfg.base_branch, version_work_branch
            )
            if version_pr and version_pr.get("state") == "MERGED":
                merge_sha = gh_get_pr_merge_commit_sha(
                    RADIUS_GH_REPO, version_pr["number"]
                )
                if merge_sha:
                    version_commit = merge_sha
                    log(f"Auto-detected version commit: {merge_sha[:7]}")

            if not version_commit:
                error(
                    "versions.yaml is already updated on main but "
                    "--version-commit was not provided and could not be "
                    "auto-detected from a merged PR.\n"
                    "Please provide --version-commit explicitly."
                )
        else:
            # Create the versions.yaml update PR to main
            log("Phase 1: Creating versions.yaml update PR on main")
            versions_content = gh_prepare_versions_yaml()
            version_files: dict[str, str] = {"versions.yaml": versions_content}

            commit_needed = True
            if gh_branch_exists(RADIUS_GH_REPO, version_work_branch):
                warn(f"SKIP: Version branch {version_work_branch} already exists")
                if gh_versions_yaml_has_version(
                    RADIUS_GH_REPO, version_work_branch, cfg.version
                ):
                    warn(
                        "SKIP: versions.yaml already committed to "
                        f"{version_work_branch}"
                    )
                    commit_needed = False
            else:
                run_mutating_api(
                    f"Create branch {version_work_branch} from {cfg.base_branch}",
                    gh_create_branch_from_ref,
                    RADIUS_GH_REPO,
                    version_work_branch,
                    cfg.base_branch,
                )

            if commit_needed:
                run_mutating_api(
                    f"Commit versions.yaml to {version_work_branch}",
                    gh_commit_files,
                    RADIUS_GH_REPO,
                    version_work_branch,
                    f"chore(release): v{cfg.version}",
                    version_files,
                )

            # Check for existing PR
            pr_info = gh_find_pr(RADIUS_GH_REPO, cfg.base_branch, version_work_branch)
            if pr_info:
                handle_existing_pr_state(step_label="versions.yaml PR", pr_info=pr_info)
            else:
                create_release_pr(
                    RADIUS_GH_REPO,
                    cfg.base_branch,
                    version_work_branch,
                    f"chore(release): v{cfg.version}",
                    f"Automated patch release versions.yaml update for v{cfg.version}.",
                )

            log(
                "Phase 1 complete. Merge the versions.yaml PR to main, "
                "then re-run to continue with cherry-pick to release branch:"
            )
            log(f"  {_build_rerun_command()}")
            return
    else:
        gh_require_commit(RADIUS_GH_REPO, version_commit)

    # Phase 2: Cherry-pick to release branch
    log(f"Phase 2: Cherry-picking to release branch {cfg.release_branch}")

    # Check if PR already exists (open or merged)
    pr_info = gh_find_pr(RADIUS_GH_REPO, cfg.release_branch, cfg.work_branch)
    if pr_info:
        state_result = handle_existing_pr_state(
            step_label="patch",
            pr_info=pr_info,
            merged_message=(f"SKIP: Patch PR already merged: {pr_info.get('url', '')}"),
            open_message=(f"SKIP: Patch PR already open: {pr_info.get('url', '')}"),
        )
        if state_result == "merged":
            run_post_merge_steps()
            log("Patch flow completed.")
            return
        if state_result == "open":
            if cfg.run_radius_verification:
                dispatch_radius_release_verification()
            log(
                "Patch flow completed. PR is awaiting review/merge. Re-run after merge:"
            )
            log(f"  {_build_rerun_command()}")
            return

    # Check if work branch already exists remotely
    if gh_branch_exists(RADIUS_GH_REPO, cfg.work_branch):
        warn(
            f"Work branch {cfg.work_branch} exists remotely "
            "- skipping local git, creating PR"
        )
        create_release_pr(
            RADIUS_GH_REPO,
            cfg.release_branch,
            cfg.work_branch,
            f"Patch release v{cfg.version}",
            f"Automated patch release PR for v{cfg.version}.",
        )
        if cfg.run_radius_verification:
            dispatch_radius_release_verification()
        log("Patch flow completed. Merge the PR, then re-run for post-merge steps:")
        log(f"  {_build_rerun_command()}")
        return

    # Need local git operations for cherry-pick
    git_checkout_and_pull_base(cfg.radius_repo, cfg.release_branch)

    # Check if local work branch already exists (partial prior run)
    result = run_cmd(
        [
            "git",
            "-C",
            cfg.radius_repo,
            "show-ref",
            "--verify",
            "--quiet",
            f"refs/heads/{cfg.work_branch}",
        ],
        check=False,
    )
    if result.returncode == 0:
        log(f"Local work branch {cfg.work_branch} already exists - reusing")
        run_mutating(
            f"Checkout existing work branch {cfg.work_branch}",
            ["git", "-C", cfg.radius_repo, "checkout", cfg.work_branch],
        )
    else:
        create_or_reset_work_branch(
            cfg.radius_repo, cfg.release_branch, cfg.work_branch
        )
        run_mutating(
            f"Cherry-pick bugfix commit {cfg.bugfix_commit} with -x --signoff",
            [
                "git",
                "-C",
                cfg.radius_repo,
                "cherry-pick",
                "-x",
                "--signoff",
                cfg.bugfix_commit,
            ],
        )
        run_mutating(
            f"Cherry-pick version commit {version_commit} with -x --signoff",
            [
                "git",
                "-C",
                cfg.radius_repo,
                "cherry-pick",
                "-x",
                "--signoff",
                version_commit,
            ],
        )

    run_mutating(
        f"Push work branch {cfg.work_branch}",
        [
            "git",
            "-C",
            cfg.radius_repo,
            "push",
            "-u",
            "origin",
            cfg.work_branch,
        ],
    )
    log(f"  Branch: https://github.com/{RADIUS_GH_REPO}/tree/{cfg.work_branch}")

    create_release_pr(
        RADIUS_GH_REPO,
        cfg.release_branch,
        cfg.work_branch,
        f"Patch release v{cfg.version}",
        f"Automated patch release PR for v{cfg.version}.",
    )

    if cfg.run_radius_verification:
        dispatch_radius_release_verification()

    log(
        "Patch flow completed. Merge the PR, then re-run to trigger "
        "post-merge workflows:"
    )
    log(f"  {_build_rerun_command()}")


# =============================================================================
# Argument parsing and entry point
# =============================================================================


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description="Cross-platform release automation for radius-project/radius.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""\
Safety model:
  Default mode is validation + dry run.
  Mutating operations require --execute.
  Each mutating operation asks for confirmation unless --yes is set.

Mode auto-detection (when --mode is omitted):
  Inferred from --version:
    X.Y.Z-rcN  -> rc mode
    X.Y.0      -> final mode
    X.Y.Z (Z>0)-> patch mode

Version auto-calculation (when --version is omitted):
  rc mode:    Looks at the newest supported channel in versions.yaml.
              If it has an RC version (X.Y.Z-rcN), increments to rcN+1.
              If it has a final version, bumps minor and starts X.Y+1.0-rc1.
  final mode: Resolves the current RC channel to its final version X.Y.0.
  patch mode: --version is always required.

Resume behaviour:
  The script checks the state of each step before executing it.
  Already-completed steps (tag exists, branch exists, PR merged, etc.)
  are skipped automatically, allowing you to safely re-run the script
  to resume an interrupted release.

Modes and local clone requirements:
  rc/final:  Uses GitHub API exclusively (no local clone needed).
             Deployment-engine tags are created via the Git Data API.
  patch:     Requires local radius repo clone for cherry-pick (default: current dir).
""",
    )
    parser.add_argument(
        "--mode",
        required=False,
        choices=["rc", "final", "patch"],
        default="",
        help="Release mode (auto-detected from --version when omitted).",
    )
    parser.add_argument(
        "--version",
        default="",
        help="Version (optional for rc/final, required for patch).",
    )
    parser.add_argument(
        "--username",
        default="",
        help="Branch name prefix (default: gh CLI logged-in user).",
    )
    parser.add_argument(
        "--radius-repo",
        default=".",
        help="Path to local radius repo clone (default: current directory).",
    )
    parser.add_argument(
        "--base-branch",
        default="main",
        help="Base branch for PRs (default: main).",
    )
    parser.add_argument(
        "--release-branch",
        default="",
        help="Override release branch (default: release/<major>.<minor>).",
    )
    parser.add_argument(
        "--work-branch",
        default="",
        help="Override work branch name.",
    )
    parser.add_argument(
        "--release-notes-file",
        default="",
        help="Local release notes file (required for final mode).",
    )
    parser.add_argument(
        "--release-notes-repo-path",
        default="",
        help="Repository path for release notes commit target (default: same as --release-notes-file).",
    )
    parser.add_argument(
        "--bugfix-commit",
        default="",
        help="Bugfix commit SHA (required for patch mode).",
    )
    parser.add_argument(
        "--version-commit",
        default="",
        help="Version commit SHA (optional for patch; auto-detected or auto-created when omitted).",
    )
    parser.add_argument(
        "--run-radius-verification",
        action="store_true",
        help="Dispatch radius release-verification workflow.",
    )
    parser.add_argument(
        "--auto-merge-pr",
        action="store_true",
        help="Queue PR auto-merge after creation.",
    )
    parser.add_argument(
        "--execute",
        action="store_true",
        help="Allow mutating commands (default: dry-run).",
    )
    parser.add_argument(
        "--yes",
        action="store_true",
        help="Skip interactive confirmations.",
    )
    return parser


def main() -> None:
    global cfg

    parser = build_parser()
    args = parser.parse_args()

    version = normalize_version(args.version) if args.version else ""

    # Infer mode from version when --mode is not specified
    mode = args.mode
    if not mode:
        if not version:
            error("Either --mode or --version must be provided")
        mode = infer_mode_from_version(version)
        log(f"Auto-detected mode from version: {mode}")

    cfg = Config(
        mode=mode,
        version=version,
        username=args.username,
        radius_repo=args.radius_repo,
        base_branch=args.base_branch,
        release_branch=args.release_branch,
        work_branch=args.work_branch,
        bugfix_commit=args.bugfix_commit,
        version_commit=args.version_commit,
        release_notes_file=args.release_notes_file,
        release_notes_repo_path=args.release_notes_repo_path,
        run_radius_verification=args.run_radius_verification,
        auto_merge_pr=args.auto_merge_pr,
        assume_yes=args.yes,
        execute_mutations=args.execute,
    )

    # --version is required for patch; optional (auto-calculated) for rc/final
    if not cfg.version and cfg.mode == "patch":
        error("--version is required for patch mode")
    if cfg.version:
        validate_version_format()

    # Check required CLI tools
    require_command("gh")
    # git is only needed for patch mode Phase 2 (local cherry-pick).
    # rc/final and patch Phase 1 use the GitHub API exclusively.
    if cfg.mode == "patch" and cfg.version_commit:
        require_command("git")

    ensure_gh_auth()

    # Auto-resolve username, display name, and email from gh CLI
    if not cfg.username:
        result = run_cmd(["gh", "api", "user", "--jq", ".login"])
        cfg.username = result.stdout.strip()
        log(f"Detected GitHub username: {cfg.username}")
    if not cfg.username:
        error("Could not determine username from gh CLI. Pass --username explicitly.")

    if not cfg.user_name or not cfg.user_email:
        user_data = gh_api_json("user")
        if not cfg.user_name:
            cfg.user_name = user_data.get("name") or cfg.username
        if not cfg.user_email:
            user_id = user_data.get("id", "")
            cfg.user_email = f"{user_id}+{cfg.username}@users.noreply.github.com"
        log(f"Sign-off identity: {cfg.user_name} <{cfg.user_email}>")

    # Auto-resolve version if not explicitly provided (rc/final only)
    if not cfg.version:
        auto_resolve_version()
        validate_version_format()

    if cfg.mode == "final" and cfg.release_notes_file:
        cfg.resolved_release_notes_repo_path = resolve_release_notes_repo_path()

    ensure_prerequisites()
    prepare_branch_names()

    log("=" * 60)
    log(f"Mode: {cfg.mode}")
    log(f"Version: v{cfg.version}")
    log(f"Radius repo (API): {RADIUS_GH_REPO}")
    if cfg.mode in ("rc", "final"):
        log(f"DE repo (API): {DE_GH_REPO}")
    if cfg.radius_repo:
        log(f"Radius repo (local): {cfg.radius_repo}")
    log(f"Release branch: {cfg.release_branch}")
    log(f"Work branch: {cfg.work_branch}")
    if cfg.mode == "final" and cfg.resolved_release_notes_repo_path:
        log(f"Release notes repo path: {cfg.resolved_release_notes_repo_path}")
    log(f"Mutations enabled: {cfg.execute_mutations}")
    log("=" * 60)

    match cfg.mode:
        case "rc":
            run_rc_flow()
        case "final":
            run_final_flow()
        case "patch":
            run_patch_flow()
        case _:
            error(f"Unhandled mode: {cfg.mode}")

    log("Done")


if __name__ == "__main__":
    main()
