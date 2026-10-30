#!/usr/bin/env python3
"""Deploy identical Bicep source to Kubernetes and ACA through Radius."""
import argparse
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import time
import urllib.request
import zipfile

HERE = Path(__file__).resolve().parent
sys.dont_write_bytecode = True
ROOT = HERE.parents[2]
RUN = Path("/home/brhamilt/delegated-provisioning-deebec8c-run")
ARTIFACTS = RUN / "portability"
spec = importlib.util.spec_from_file_location(
    "local_demo", HERE.parent / "delegated-provisioning/demo.py"
)
local_demo = importlib.util.module_from_spec(spec)
spec.loader.exec_module(local_demo)
api, wait, command, RAD = local_demo.api, local_demo.wait, local_demo.command, local_demo.RAD
os.environ["BICEP"] = "/home/brhamilt/.local/bin/bicep"
SUBSCRIPTION = "2b04a171-9c7d-495f-9b99-96cefa291b05"
AZURE_GROUP = "rg-radius-aca-proto-28500d6a"
MANAGED_ENVIRONMENT = (
    f"/subscriptions/{SUBSCRIPTION}/resourceGroups/{AZURE_GROUP}"
    "/providers/Microsoft.App/managedEnvironments/aca-proto-28500d6a"
)
POWERSHELL = "/mnt/c/Program Files/PowerShell/7/pwsh.exe"
NATIVE_HELPER = (
    "D:\\cp\\wt\\radius\\brooke-hamilton-studious-fortnight\\"
    "test\\experimental\\delegated-provisioning-portability\\aca-native.ps1"
)


def scope(platform):
    return "/planes/radius/local/resourceGroups/delegation-portable-" + platform


def resource_id(platform, resource_type, name):
    return scope(platform) + "/providers/" + resource_type + "/" + name


def package(platform):
    if platform == "aca-bicep":
        source_file = HERE / "aca-bicep/container-app.bicep"
        tag = hashlib.sha256(source_file.read_bytes()).hexdigest()[:16]
        command(
            RAD, "bicep", "publish", "--file", source_file,
            "--target", "br:localhost:5568/recipes/aca-delegated:" + tag,
            "--plain-http",
        )
        request = urllib.request.Request(
            "http://127.0.0.1:5568/v2/recipes/aca-delegated/manifests/" + tag,
            headers={"Accept": "application/vnd.oci.image.manifest.v1+json"},
        )
        with urllib.request.urlopen(request, timeout=30) as response:
            digest = response.headers["Docker-Content-Digest"]
        registry = json.loads(command("docker", "inspect", "delegated-registry-deebec8c"))[0]
        address = registry["NetworkSettings"]["Networks"]["delegated-net-deebec8c"]["IPAddress"]
        return f"{address}:5000/recipes/aca-delegated@{digest}", digest.removeprefix("sha256:")
    image = json.loads(command("docker", "inspect", "delegated-recipes-deebec8c"))[0]
    address = image["NetworkSettings"]["Networks"]["delegated-net-deebec8c"]["IPAddress"]
    path = RUN / "modules" / ("portability-" + platform + ".zip")
    mirror = RUN / "tools/provider-mirror" if platform == "kubernetes" else ARTIFACTS / "providers"
    provider = "kubernetes" if platform == "kubernetes" else "azurerm"
    with zipfile.ZipFile(path, "w") as archive:
        def add(name, data, mode=0o100644):
            entry = zipfile.ZipInfo(name, (2026, 1, 1, 0, 0, 0))
            entry.compress_type = zipfile.ZIP_DEFLATED
            entry.external_attr = mode << 16
            archive.writestr(entry, data)
        add("main.tf", (HERE / platform / "main.tf").read_bytes())
        for source in sorted(mirror.rglob("*")):
            if source.is_file():
                add("providers/" + str(source.relative_to(mirror)), source.read_bytes(), source.stat().st_mode & 0xFFFF)
        add("terraform.rc", (
            "provider_installation {\n  filesystem_mirror {\n"
            '    path = ".terraform/modules/default/providers"\n'
            f'    include = ["registry.terraform.io/hashicorp/{provider}"]\n'
            "  }\n}\n"
        ).encode())
    digest = hashlib.sha256(path.read_bytes()).hexdigest()
    pinned = path.with_name(digest + ".zip")
    if not pinned.exists():
        pinned.write_bytes(path.read_bytes())
    return f"http://{address}/{digest}.zip", digest


def configure(platform):
    group = scope(platform)
    api("PUT", group, {"location": "global", "properties": {}}, version="2023-10-01-preview")
    namespace = "portable-" + platform
    if not command("kubectl", "get", "namespace", namespace, "--ignore-not-found"):
        command("kubectl", "create", "namespace", namespace)
    source, digest = package(platform)
    pack = resource_id(platform, "Radius.Core/recipePacks", "delegation-" + digest[:12])
    target_recipe = {
        "kind": "bicep" if platform == "aca-bicep" else "terraform", "source": source,
        "acceptsDelegationFrom": ["Radius.Compute/routes"],
    }
    if platform.startswith("aca"):
        target_recipe["parameters"] = {
            "managedEnvironmentId": MANAGED_ENVIRONMENT,
        }
    if platform == "aca-bicep":
        target_recipe["plainHttp"] = True
    api("PUT", pack, {"location": "global", "properties": {"recipes": {
        "Radius.Compute/containers": target_recipe,
        "Radius.Compute/routes": {
            "kind": "delegated",
            "target": {
                "resourceType": "Radius.Compute/containers",
                "idsFrom": "$.rules[*].destinationContainer.resourceId",
            },
        },
    }}})
    settings = resource_id(platform, "Radius.Core/terraformSettings", "offline")
    api("PUT", settings, {"location": "global", "properties": {
        "env": {
            "TF_CLI_CONFIG_FILE": ".terraform/modules/default/terraform.rc",
            "ARM_RESOURCE_PROVIDER_REGISTRATIONS": "none",
        }
    }})
    providers = {"kubernetes": {"namespace": namespace}}
    if platform.startswith("aca"):
        providers["azure"] = {"subscriptionId": SUBSCRIPTION, "resourceGroupName": AZURE_GROUP}
    environment = resource_id(platform, "Radius.Core/environments", "demo")
    api("PUT", environment, {"location": "global", "properties": {
        "recipePacks": [pack], "terraformSettings": settings, "providers": providers,
    }})
    workspace = "portable-" + platform
    command(
        RAD, "workspace", "create", "kubernetes", workspace,
        "--context", "kind-delegated-deebec8c",
        "--group", "delegation-portable-" + platform, "--environment", "demo", "--force",
    )
    return workspace


def read_aca(app_name, action="show"):
    completed = subprocess.run(
        [POWERSHELL, "-NoProfile", "-File", NATIVE_HELPER, "-Name", app_name, "-Action", action],
        cwd="/mnt/c/Windows/Temp", text=True, capture_output=True, check=False,
    )
    if completed.returncode:
        raise RuntimeError(completed.stderr)
    if action == "logs":
        return "\n".join(json.loads(completed.stdout))
    return json.loads(completed.stdout)


def native(platform, name, action="show"):
    if platform == "kubernetes":
        if action == "logs":
            return command("kubectl", "-n", "portable-kubernetes", "logs", "deployment/delegated-peer", "--tail=50", "--timestamps")
        kind = "service" if name == "ingress" else "deployment"
        actual = "delegated-server" if name == "ingress" else "delegated-" + name
        output = command("kubectl", "-n", "portable-kubernetes", "get", kind, actual, "--ignore-not-found", "-o", "json")
        return json.loads(output) if output else None
    resource = api("GET", resource_id(platform, "Radius.Compute/containers", name))
    owned = resource["properties"]["status"]["outputResources"]
    assert len(owned) == 1
    app_name = owned[0]["id"].rsplit("/", 1)[1]
    return read_aca(app_name, action)


def snapshot(platform, label, expected_port):
    target = api("GET", resource_id(platform, "Radius.Compute/containers", "server"))
    route = api("GET", resource_id(platform, "Radius.Compute/routes", "ingress"), missing_ok=True)
    target_native = native(platform, "server")
    reconciliation = target["properties"]["status"]["reconciliation"]
    assert reconciliation["desiredRevision"] == reconciliation["appliedRevision"]
    if platform == "kubernetes":
        ingress_native = native(platform, "ingress")
        ports = [entry["port"] for entry in ingress_native["spec"]["ports"]] if ingress_native else []
        assert ports == ([] if expected_port is None else [expected_port]), ports
    else:
        assert target_native["systemData"]["createdBy"] == "3d47c861-1d1c-4a51-9f3c-9ad6508e34df"
        ingress_native = target_native["ingress"]
        if expected_port is None:
            assert ingress_native is None, ingress_native
        else:
            assert ingress_native["transport"].lower() == "tcp"
            assert ingress_native["targetPort"] == expected_port
            assert ingress_native["exposedPort"] == expected_port
            assert ingress_native["external"] is False
    if route:
        assert route["properties"]["listener"]["port"] == expected_port
        assert not route["properties"].get("status", {}).get("outputResources")
    record = {
        "platform": platform, "phase": label,
        "appDefinitionSHA256": hashlib.sha256((HERE / "app.bicep").read_bytes()).hexdigest(),
        "target": target, "route": route,
        "nativeTarget": target_native, "nativeIngress": ingress_native,
    }
    (ARTIFACTS / platform / (label + ".json")).write_text(json.dumps(record, indent=2) + "\n")
    print(f"PASS {platform} {label}: native and Radius listener port={expected_port}", flush=True)
    return record


def peer_event(line, platform):
    if platform == "kubernetes":
        timestamp, _, message = line.partition(" ")
        stamp = datetime.fromisoformat(timestamp.replace("Z", "+00:00"))
    else:
        outer = json.loads(line)
        stamp = datetime.fromisoformat(outer["TimeStamp"].replace("Z", "+00:00"))
        message = outer["Log"]
        if message.startswith(("F ", "P ")):
            message = message[2:]
    return stamp, json.loads(message)


def probe(platform, port):
    deadline = time.monotonic() + 180
    started = datetime.now(timezone.utc)
    latest = ""
    while time.monotonic() < deadline:
        latest = native(platform, "peer", "logs")
        for line in latest.splitlines():
            try:
                stamp, record = peer_event(line, platform)
                if stamp >= started and record.get("port") == port and record.get("ok") is True:
                    (ARTIFACTS / platform / f"traffic-{port}.log").write_text(latest)
                    print(f"PASS {platform} live peer traffic on TCP {port}", flush=True)
                    return
            except (json.JSONDecodeError, ValueError):
                continue
        time.sleep(5)
    raise RuntimeError(f"No successful {platform} peer traffic on {port}: {latest}")


def put(platform, resource_type, name, properties):
    path = ARTIFACTS / platform / (name + "-request.json")
    path.write_text(json.dumps({"properties": properties}, indent=2))
    command(RAD, "resource", "create", resource_type, name, "-f", path, "--workspace", "portable-" + platform)


def reset(platform):
    cloud_names = []
    for resource_type, name in (
        ("Radius.Compute/routes", "ingress"),
        ("Radius.Compute/containers", "peer"),
        ("Radius.Compute/containers", "server"),
    ):
        existing = api("GET", resource_id(platform, resource_type, name), missing_ok=True)
        if existing is None:
            continue
        if platform.startswith("aca"):
            cloud_names.extend(
                output["id"].rsplit("/", 1)[1]
                for output in existing["properties"].get("status", {}).get("outputResources", [])
            )
        command(RAD, "resource", "delete", resource_type, name, "--yes", "--workspace", "portable-" + platform)
        wait(resource_id(platform, resource_type, name), deleting=True)
    application = resource_id(platform, "Radius.Core/applications", "portable-demo")
    if api("GET", application, missing_ok=True):
        api("DELETE", application)
    if platform.startswith("aca"):
        for name in cloud_names:
            assert read_aca(name).get("absent") is True
    elif command("kubectl", "get", "namespace", "portable-kubernetes", "--ignore-not-found"):
        assert native(platform, "server") is None
        assert native(platform, "peer") is None
        assert native(platform, "ingress") is None
    (ARTIFACTS / platform / "reset.json").write_text(json.dumps({
        "platform": platform, "scope": scope(platform),
        "nativeAppsConfirmedAbsent": cloud_names, "passed": True,
    }, indent=2) + "\n")
    print(f"PASS {platform} reset: owned native workloads removed through Radius", flush=True)


def run(platform):
    destination = ARTIFACTS / platform
    destination.mkdir(exist_ok=True)
    reset(platform)
    workspace = configure(platform)
    print(f"$ rad deploy app.bicep --workspace {workspace}", flush=True)
    output = command(RAD, "deploy", HERE / "app.bicep", "--workspace", workspace)
    (destination / "deployment.log").write_text(output)
    first = snapshot(platform, "01-shared-app", 80)
    native_identity = (
        first["nativeTarget"]["metadata"]["uid"] if platform == "kubernetes"
        else (first["nativeTarget"]["id"], first["nativeTarget"]["systemData"]["createdAt"])
    )
    probe(platform, 80)
    properties = dict(first["target"]["properties"])
    for key in ("status", "provisioningState", "recipe"):
        properties.pop(key, None)
    properties["containers"]["web"]["image"] = "nginx:1.28-alpine"
    print(f"$ rad resource create Radius.Compute/containers server --workspace {workspace}", flush=True)
    put(platform, "Radius.Compute/containers", "server", properties)
    changed = snapshot(platform, "02-owner-only-update", 80)
    containers = (
        changed["nativeTarget"]["spec"]["template"]["spec"]["containers"]
        if platform == "kubernetes" else changed["nativeTarget"]["containers"]
    )
    assert containers[0]["image"] == "nginx:1.28-alpine"
    route = dict(first["route"]["properties"])
    for key in ("status", "provisioningState", "listener"):
        route.pop(key, None)
    route["rules"][0]["destinationContainer"]["containerPort"] = 8080
    print(f"$ rad resource create Radius.Compute/routes ingress --workspace {workspace}", flush=True)
    put(platform, "Radius.Compute/routes", "ingress", route)
    snapshot(platform, "03-route-only-update", 8080)
    probe(platform, 8080)
    command(RAD, "resource", "delete", "Radius.Compute/routes", "ingress", "--yes", "--workspace", workspace)
    final = snapshot(platform, "04-route-delete", None)
    final_identity = (
        final["nativeTarget"]["metadata"]["uid"] if platform == "kubernetes"
        else (final["nativeTarget"]["id"], final["nativeTarget"]["systemData"]["createdAt"])
    )
    assert final_identity == native_identity, "native workload was recreated"
    put(platform, "Radius.Compute/routes", "ingress", route)
    restored = snapshot(platform, "05-route-recreate", 8080)
    restored_identity = (
        restored["nativeTarget"]["metadata"]["uid"] if platform == "kubernetes"
        else (restored["nativeTarget"]["id"], restored["nativeTarget"]["systemData"]["createdAt"])
    )
    assert restored_identity == native_identity, "route recreation replaced the workload"
    probe(platform, 8080)
    print(f"PASS {platform}: same application source and automatic single-writer lifecycle", flush=True)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("platform", choices=["kubernetes", "aca", "aca-bicep"])
    parser.add_argument("--reset", action="store_true")
    args = parser.parse_args()
    ARTIFACTS.mkdir(exist_ok=True)
    log = (ARTIFACTS / "proxy.log").open("w")
    proxy = subprocess.Popen(["kubectl", "proxy", "--port=18586"], stdout=log, stderr=subprocess.STDOUT)
    try:
        for _ in range(50):
            if proxy.poll() is not None:
                raise RuntimeError("Owned API proxy could not start.")
            try:
                urllib.request.urlopen("http://127.0.0.1:18586/version", timeout=1).close()
                break
            except OSError:
                time.sleep(0.2)
        else:
            raise RuntimeError("Owned API proxy readiness timed out.")
        (ARTIFACTS / args.platform).mkdir(exist_ok=True)
        if args.reset:
            reset(args.platform)
        else:
            run(args.platform)
    finally:
        proxy.terminate()
        proxy.wait(timeout=10)
        log.close()


if __name__ == "__main__":
    main()
