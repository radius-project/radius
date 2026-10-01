// @ts-check

import { mkdir, readFile, writeFile } from "node:fs/promises";
import { createHash } from "node:crypto";
import path from "node:path";
import { bicepExtensionLockName } from "./release-bicep-extensions.mjs";

import {
  waitForRelease,
  waitForReleaseState
} from "./release-reconciliation.mjs";

/** @param {unknown} value */
function asBuffer(value) {
  if (Buffer.isBuffer(value)) {
    return value;
  }
  if (value instanceof ArrayBuffer) {
    return Buffer.from(value);
  }
  if (ArrayBuffer.isView(value)) {
    return Buffer.from(value.buffer, value.byteOffset, value.byteLength);
  }
  return Buffer.from(String(value));
}

/** @param {any} github @param {string} owner @param {string} repo @param {number} releaseID */
async function listAssets(github, owner, repo, releaseID) {
  return github.paginate(github.rest.repos.listReleaseAssets, {
    owner,
    repo,
    release_id: releaseID,
    per_page: 100
  });
}

/** @param {any} github @param {string} owner @param {string} repo @param {number} assetID */
async function downloadAsset(github, owner, repo, assetID) {
  const response = await github.request(
    "GET /repos/{owner}/{repo}/releases/assets/{asset_id}",
    {
      owner,
      repo,
      asset_id: assetID,
      headers: { accept: "application/octet-stream" }
    }
  );
  return asBuffer(response.data);
}

/** @param {any[]} assets @param {string} name */
function findAsset(assets, name) {
  const matches = assets.filter((asset) => asset.name === name);
  if (matches.length > 1) {
    throw new Error(`Release contains multiple assets named ${name}`);
  }
  return matches[0];
}

/** @param {any} github @param {string} owner @param {string} repo @param {any} asset @param {import("./release-reconciliation.mjs").ReleaseReadOptions} options */
function downloadUploadedAsset(github, owner, repo, asset, options) {
  return waitForReleaseState(
    async () => {
      try {
        return await downloadAsset(github, owner, repo, asset.id);
      } catch (error) {
        if (error.status !== 404) throw error;
        return undefined;
      }
    },
    {
      ...options,
      description: `Uploaded release asset ${asset.name} (ID ${asset.id})`
    }
  );
}

export async function readBicepExtensionLock(github, tag) {
  const owner = "radius-project";
  const repo = "radius";
  const releases = await github.paginate(github.rest.repos.listReleases, {
    owner,
    repo,
    per_page: 100
  });
  const matches = releases.filter((release) => release.tag_name === tag);
  if (matches.length > 1) throw new Error(`Multiple releases for ${tag}`);
  if (!matches.length) return undefined;
  const release = matches[0];
  const asset = findAsset(
    await listAssets(github, owner, repo, release.id),
    bicepExtensionLockName
  );
  if (!asset) {
    if (!release.draft)
      throw new Error("Published GHCR release is missing its Bicep lock");
    return undefined;
  }
  return JSON.parse(
    (await downloadAsset(github, owner, repo, asset.id)).toString("utf8")
  );
}

/** @param {Buffer} data @param {string} name */
export function verifySpdxDocument(data, name) {
  let document;
  try {
    document = JSON.parse(data.toString("utf8"));
  } catch (error) {
    throw new Error(`Release SBOM ${name} is not valid JSON`, { cause: error });
  }
  const creators = document?.creationInfo?.creators;
  const packages = document?.packages;
  if (
    typeof document !== "object" ||
    document === null ||
    !/^SPDX-2\.\d+$/.test(document.spdxVersion) ||
    document.SPDXID !== "SPDXRef-DOCUMENT" ||
    document.dataLicense !== "CC0-1.0" ||
    typeof document.name !== "string" ||
    typeof document.documentNamespace !== "string" ||
    !document.documentNamespace.startsWith("https://") ||
    typeof document.creationInfo?.created !== "string" ||
    !Array.isArray(creators) ||
    !creators.some((creator) => /^Tool: syft-/.test(creator)) ||
    !Array.isArray(packages) ||
    packages.length === 0 ||
    !packages.every(
      (entry) =>
        entry !== null &&
        typeof entry === "object" &&
        !Array.isArray(entry) &&
        typeof entry.SPDXID === "string" &&
        typeof entry.name === "string" &&
        typeof entry.downloadLocation === "string"
    ) ||
    !Array.isArray(document.relationships)
  ) {
    throw new Error(`Release SBOM ${name} is not a valid SPDX document`);
  }
}

async function deleteStarterAsset(github, owner, repo, release, asset) {
  if (asset?.state !== "starter") {
    return false;
  }
  if (!release.draft) {
    throw new Error(
      `Cannot replace incomplete asset ${asset.name} on a published release`
    );
  }
  await github.rest.repos.deleteReleaseAsset({
    owner,
    repo,
    asset_id: asset.id
  });
  return true;
}

async function uploadAssetData(
  github,
  owner,
  repo,
  release,
  name,
  data,
  reconciliation
) {
  const maxUploadAttempts = 3;
  let response;
  for (let attempt = 1; attempt <= maxUploadAttempts; attempt++) {
    try {
      response = await github.rest.repos.uploadReleaseAsset({
        owner,
        repo,
        release_id: release.id,
        name,
        headers: {
          "content-type":
            name.endsWith(".json") ? "application/json" : "text/plain",
          "content-length": data.length
        },
        data
      });
      break;
    } catch (error) {
      const uncertain = await waitForReleaseState(
        async () =>
          findAsset(await listAssets(github, owner, repo, release.id), name),
        {
          ...reconciliation,
          description: `Release asset ${name} after upload to release ${release.id}`,
          cause: error
        }
      );
      if (await deleteStarterAsset(github, owner, repo, release, uncertain)) {
        if (attempt === maxUploadAttempts) {
          throw error;
        }
        continue;
      }
      const remote = await downloadUploadedAsset(
        github,
        owner,
        repo,
        uncertain,
        reconciliation
      );
      if (Buffer.compare(data, remote) !== 0) {
        throw error;
      }
      response = { data: uncertain };
      break;
    }
  }
  const uploaded = response.data;
  if (uploaded.name !== name) {
    throw new Error(
      `Uploaded release asset ${uploaded.id} is not named ${name}`
    );
  }
  const remote = await downloadUploadedAsset(
    github,
    owner,
    repo,
    uploaded,
    reconciliation
  );
  if (Buffer.compare(data, remote) !== 0) {
    throw new Error(`Uploaded release asset ${name} failed verification`);
  }
  return response.data;
}

/** @param {any} github @param {any} core @param {string} owner @param {string} repo @param {any} release */
async function downloadFiles(github, core, owner, repo, release) {
  const names = JSON.parse(core.getInput("NAMES", { required: true }));
  const outputDirectory = core.getInput("OUTPUT_DIR", { required: true });
  const optional = core.getInput("OPTIONAL") === "true";
  if (!Array.isArray(names) || names.some((name) => typeof name !== "string")) {
    throw new Error("NAMES must be a JSON array of strings");
  }

  const assets = await listAssets(github, owner, repo, release.id);
  const missing = names.filter((name) => !findAsset(assets, name));
  if (
    missing.length > 0 &&
    (!optional || missing.includes(bicepExtensionLockName))
  ) {
    throw new Error(`Release is missing assets: ${missing.join(", ")}`);
  }
  if (missing.length > 0) {
    core.setOutput("all_found", "false");
    return;
  }

  await mkdir(outputDirectory, { recursive: true });
  for (const name of names) {
    const asset = findAsset(assets, name);
    const data = await downloadAsset(github, owner, repo, asset.id);
    await writeFile(path.join(outputDirectory, name), data);
  }
  core.setOutput("all_found", "true");
}

/** @param {any} github @param {any} core @param {string} owner @param {string} repo @param {any} release @param {string} file @param {import("./release-reconciliation.mjs").ReleaseReadOptions} reconciliation */
async function uploadFile(
  github,
  core,
  owner,
  repo,
  release,
  file,
  reconciliation
) {
  const name = path.basename(file);
  const data = await readFile(file);
  const immutable =
    name === bicepExtensionLockName || core.getInput("IMMUTABLE") === "true";
  let assets = await listAssets(github, owner, repo, release.id);
  let existing = findAsset(assets, name);
  if (await deleteStarterAsset(github, owner, repo, release, existing)) {
    existing = undefined;
  }

  if (existing) {
    const remote = await downloadAsset(github, owner, repo, existing.id);
    if (Buffer.compare(data, remote) === 0) {
      core.setOutput("asset_id", String(existing.id));
      core.setOutput("reused", "true");
      return;
    }
    if (immutable) {
      throw new Error(
        `Immutable release asset ${name} differs from local data`
      );
    }
    if (!release.draft) {
      throw new Error(
        `Published release asset ${name} differs from local data`
      );
    }
    await github.rest.repos.deleteReleaseAsset({
      owner,
      repo,
      asset_id: existing.id
    });
  } else if (!release.draft) {
    throw new Error(`Cannot add ${name} to a published release`);
  }

  const uploaded = await uploadAssetData(
    github,
    owner,
    repo,
    release,
    name,
    data,
    reconciliation
  );
  core.setOutput("asset_id", String(uploaded.id));
  core.setOutput("reused", "false");
}

/** @param {any} github @param {any} core @param {string} owner @param {string} repo @param {any} release @param {import("./release-reconciliation.mjs").ReleaseReadOptions} reconciliation */
async function uploadFiles(github, core, owner, repo, release, reconciliation) {
  const filesInput = core.getInput("FILES");
  const files =
    filesInput ?
      JSON.parse(filesInput)
    : [core.getInput("FILE", { required: true })];
  if (!Array.isArray(files) || files.some((file) => typeof file !== "string")) {
    throw new Error("FILES must be a JSON array of strings");
  }
  for (const file of files) {
    await uploadFile(github, core, owner, repo, release, file, reconciliation);
  }
  core.setOutput("uploaded_files", String(files.length));
}

/** @param {any} github @param {any} core @param {string} owner @param {string} repo @param {any} release @param {boolean} normalize @param {import("./release-reconciliation.mjs").ReleaseReadOptions} reconciliation */
async function reconcileCliAssets(
  github,
  core,
  owner,
  repo,
  release,
  normalize,
  reconciliation
) {
  const targetsFile = core.getInput("TARGETS_FILE", { required: true });
  const targets = JSON.parse(await readFile(targetsFile, "utf8"));
  const names = targets.cliAssets.map((asset) => asset.name);
  let assets = await listAssets(github, owner, repo, release.id);

  for (const name of names) {
    const binaryAsset = findAsset(assets, name);
    const checksumAsset = findAsset(assets, `${name}.sha256`);
    if (!binaryAsset) {
      throw new Error(`Release is missing ${name}`);
    }
    const binary = await downloadAsset(github, owner, repo, binaryAsset.id);
    const actual = createHash("sha256").update(binary).digest("hex");
    const expected = Buffer.from(`${actual} *${name}\n`);
    if (!checksumAsset) {
      if (!normalize || !release.draft) {
        throw new Error(`Release is missing ${name}.sha256`);
      }
      await uploadAssetData(
        github,
        owner,
        repo,
        release,
        `${name}.sha256`,
        expected,
        reconciliation
      );
      continue;
    }
    const checksum = (
      await downloadAsset(github, owner, repo, checksumAsset.id)
    )
      .toString("utf8")
      .trim();
    const declared = checksum.split(/\s+/)[0];
    if (!/^[0-9a-f]{64}$/.test(declared)) {
      throw new Error(`Release checksum has an invalid format: ${name}`);
    }
    if (actual !== declared) {
      throw new Error(`Release checksum mismatch: ${name}`);
    }
    if (normalize && checksum !== expected.toString("utf8").trim()) {
      if (!release.draft) {
        throw new Error(`Cannot normalize ${name}.sha256 after publication`);
      }
      await github.rest.repos.deleteReleaseAsset({
        owner,
        repo,
        asset_id: checksumAsset.id
      });
      await uploadAssetData(
        github,
        owner,
        repo,
        release,
        `${name}.sha256`,
        expected,
        reconciliation
      );
    } else if (!normalize && checksum !== expected.toString("utf8").trim()) {
      throw new Error(`Release checksum is not Radius-compatible: ${name}`);
    }
  }
  core.setOutput("verified_assets", String(names.length));
}

/** @param {any} github @param {any} core @param {string} owner @param {string} repo @param {any} release */
async function verifySbomAssets(github, core, owner, repo, release) {
  const targetsFile = core.getInput("TARGETS_FILE", { required: true });
  const targets = JSON.parse(await readFile(targetsFile, "utf8"));
  const cliAssets = /** @type {{name: string}[]} */ (targets.cliAssets);
  const names = cliAssets.map((asset) => `${asset.name}.sbom.json`).sort();
  const assets = /** @type {{id: number, name: string}[]} */ (
    await listAssets(github, owner, repo, release.id)
  );
  const actual = assets
    .map((asset) => asset.name)
    .filter((name) => name.endsWith(".sbom.json"))
    .sort();
  if (JSON.stringify(actual) !== JSON.stringify(names)) {
    throw new Error("Release SBOM assets do not match the expected set");
  }
  for (const name of names) {
    const asset = findAsset(assets, name);
    const data = await downloadAsset(github, owner, repo, asset.id);
    verifySpdxDocument(data, name);
  }
  core.setOutput("verified_sboms", String(names.length));
}

/** @param {{github: any} & import("./release-reconciliation.mjs").ReleaseReadOptions} options */
export default async function releaseAssets({ github, core, sleep }) {
  const owner = core.getInput("OWNER", { required: true });
  const repo = core.getInput("REPO", { required: true });
  const tag = core.getInput("TAG", { required: true });
  const mode = core.getInput("MODE", { required: true });
  const reconciliation = { core, sleep };
  const release = await waitForRelease(
    github,
    owner,
    repo,
    tag,
    reconciliation
  );

  if (mode === "download") {
    await downloadFiles(github, core, owner, repo, release);
    return;
  }
  if (mode === "upload") {
    await uploadFiles(github, core, owner, repo, release, reconciliation);
    return;
  }
  if (mode === "verify-cli") {
    await reconcileCliAssets(
      github,
      core,
      owner,
      repo,
      release,
      false,
      reconciliation
    );
    return;
  }
  if (mode === "normalize-cli") {
    await reconcileCliAssets(
      github,
      core,
      owner,
      repo,
      release,
      true,
      reconciliation
    );
    return;
  }
  if (mode === "verify-sboms") {
    await verifySbomAssets(github, core, owner, repo, release);
    return;
  }
  throw new Error(`Unsupported release asset mode: ${mode}`);
}
