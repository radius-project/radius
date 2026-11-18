import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

test("packaged Radius types compile a resource-group role assignment", (t) => {
  const directory = mkdtempSync(join(tmpdir(), "radius-rbac-bicep-"));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  const bicep = process.env.BICEP_PATH || "bicep";
  execFileSync(bicep, [
    "publish-extension",
    fileURLToPath(
      new URL(
        "../../../hack/bicep-types-radius/generated/index.json",
        import.meta.url
      )
    ),
    "--target",
    join(directory, "radius.tgz")
  ]);
  writeFileSync(
    join(directory, "bicepconfig.json"),
    JSON.stringify({ extensions: { radius: "./radius.tgz" } })
  );
  const source = join(directory, "roleAssignment.bicep");
  writeFileSync(
    source,
    `extension radius

targetScope = 'resourceGroup'

resource customRole 'Radius.Core/roleDefinitions@2025-08-01-preview' = {
  name: 'application-reader'
  properties: {
    permissions: [
      'Radius.Core/applications/read'
    ]
  }
}

resource roleAssignment 'Radius.Core/roleAssignments@2025-08-01-preview' = {
  name: guid('team-a', 'application-reader', 'group', 'kubernetes:test-cluster', 'team-a-devs')
  properties: {
    roleDefinitionId: customRole.id
    principal: {
      type: 'group'
      issuer: 'kubernetes:test-cluster'
      subject: 'team-a-devs'
    }
  }
}
`
  );
  const compiled = spawnSync(bicep, ["build", source, "--stdout"], {
    encoding: "utf8"
  });
  assert.ifError(compiled.error);
  assert.equal(compiled.status, 0, compiled.stderr);
  assert.equal(
    compiled.stderr,
    "",
    "Bicep compilation must have no diagnostics"
  );
  const template = JSON.parse(compiled.stdout);
  assert.equal(
    template.resources.roleAssignment.type,
    "Radius.Core/roleAssignments@2025-08-01-preview"
  );
  assert.equal(
    template.resources.customRole.type,
    "Radius.Core/roleDefinitions@2025-08-01-preview"
  );
  assert.equal(
    template.resources.roleAssignment.properties.properties.principal.type,
    "group"
  );
});
