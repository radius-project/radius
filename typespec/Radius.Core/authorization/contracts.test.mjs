import assert from "node:assert/strict";
import { readFileSync, readdirSync } from "node:fs";
import test from "node:test";

const root = new URL("../../../", import.meta.url);
const examplesDir = new URL("examples/2025-08-01-preview/", import.meta.url);
const read = (url) => readFileSync(url, "utf8");
const json = (url) => JSON.parse(read(url));
const examples = readdirSync(examplesDir)
  .filter((file) => file.endsWith(".json"))
  .map((file) => ({ file, ...json(new URL(file, examplesDir)) }));

test("examples preserve the requested role and assignment shapes", async (t) => {
  assert.equal(examples.length, 3);
  for (const example of examples) {
    await t.test(example.file, () => {
      const { parameters } = example;
      const response = example.responses["201"].body;
      assert.deepEqual(example.responses["200"].body, response);
      assert.equal(parameters["api-version"], "2025-08-01-preview");
      assert.ok(parameters.scope.startsWith("planes/"));
      assert.equal(parameters.scope.endsWith("/"), false);
      assert.equal(
        response.name,
        parameters.roleDefinitionName ?? parameters.roleAssignmentName,
      );
      assert.equal(
        response.id,
        `/${parameters.scope}/providers/${response.type}/${response.name}`,
      );
      const request = parameters.resource.properties;
      if (response.type === "Radius.Core/roleDefinitions") {
        assert.equal(response.properties.roleType, "Custom");
        assert.equal("roleType" in request, false);
        assert.deepEqual(response.properties.permissions, request.permissions);
        assert.ok(
          request.permissions.every((permission) => !permission.includes("*")),
        );
      } else {
        assert.equal(response.type, "Radius.Core/roleAssignments");
        assert.equal(response.properties.scope, `/${parameters.scope}`);
        for (const field of ["scope", "managedBy", "createdBy"]) {
          assert.equal(
            field in request,
            false,
            `${field} must be server-owned`,
          );
        }
        assert.deepEqual(request.principal, response.properties.principal);
        assert.deepEqual(
          Object.fromEntries(
            ["type", "issuer", "subject"].map((key) => [
              key,
              request.principal[key],
            ]),
          ),
          {
            type: "group",
            issuer: "kubernetes:6c1e9a8e-1c21-4774-a261-d760b21ad530",
            subject: "team-a-devs",
          },
        );
        assert.equal(
          response.properties.roleDefinitionId,
          request.roleDefinitionId,
        );
      }
    });
  }
  const environment = examples.find(({ file }) => file.includes("Environment"));
  assert.equal(
    environment.parameters.scope,
    "planes/radius/local/resourceGroups/platform/providers/Radius.Core/environments/staging",
  );
  assert.match(
    environment.responses["201"].body.id,
    /\/environments\/staging\/providers\/Radius[.]Core\/roleAssignments\//,
  );
});

test("entry points keep authorization out of Applications RP generation", () => {
  const applicationEntry = read(new URL("../main.tsp", import.meta.url));
  assert.doesNotMatch(applicationEntry, /import\s+["'][^"']*authorization/);
  assert.match(
    read(new URL("main.tsp", import.meta.url)),
    /import "\.\.\/authorization[.]tsp"/,
  );
  const bicepEntry = read(new URL("../bicep/main.tsp", import.meta.url));
  assert.match(bicepEntry, /import "\.\.\/main[.]tsp"/);
  assert.match(bicepEntry, /import "\.\.\/authorization[.]tsp"/);
  const config = read(new URL("tspconfig.yaml", import.meta.url));
  assert.match(config, /pkg\/ucp\/api\/authorization\/v20250801preview/);
  assert.doesNotMatch(config, /pkg\/corerp/);
});

test("scope syntax preserves ARM IDs and permissions exclude wildcards", () => {
  const source = read(new URL("../authorization.tsp", import.meta.url));
  const scalarPattern = (name) => {
    const match = source.match(
      new RegExp(`@pattern\\("([^"]+)"\\)\\s*scalar ${name} extends string`),
    );
    assert.ok(match, `missing scalar constraint: ${name}`);
    return new RegExp(JSON.parse(`"${match[1]}"`));
  };
  const scope = scalarPattern("AuthorizationScope");
  const plane = "/planes/radius/local";
  const group = `${plane}/resourceGroups/team-a`;
  for (const value of [
    "/",
    plane,
    group,
    "/planes/aws/aws-account",
    "/planes/azure/azure-subscription",
  ]) {
    assert.ok(scope.test(value), value);
  }
  for (const type of [
    "Radius.Core/environments",
    "Radius.Core/recipePacks",
    "Radius.Core/applications",
    "Applications.Core/environments",
    "Applications.Core/applications",
  ]) {
    const value = `${group}/providers/${type}/one`;
    assert.ok(scope.test(value), value);
    assert.ok(scope.test(value.toLowerCase()), value.toLowerCase());
  }
  for (const value of [
    "",
    `${group}/`,
    `${group}//providers/Radius.Core/environments/one`,
    `${group}/providers/Radius.Core/environments/*`,
    `${group}/providers/Radius.Core/environments/white space`,
  ]) {
    assert.equal(scope.test(value), false, value);
  }
  const permission = scalarPattern("PermissionName");
  for (const value of [
    "Radius.Core/applications/read",
    "Radius.Core/environments/use/action",
    "System.Resources/resourceProviders/resourceTypes/joinPermissionSet/action",
    "Radius.Data/my-sql/read",
  ]) {
    assert.ok(permission.test(value), value);
  }
  for (const value of [
    "*",
    "Radius.Core/*",
    "Radius.Core/applications/*",
    "Radius.Core/applications/read ",
  ]) {
    assert.equal(permission.test(value), false, value);
  }
  const path = scalarPattern("AuthorizationScopePath");
  assert.ok(path.test(group.slice(1)));
  assert.equal(path.test(group), false);
  assert.equal(path.test(`${group.slice(1)}/`), false);
});

test("Bicep merges existing resources, preserves extensions and singleton, and excludes read-only APIs", () => {
  const types = json(
    new URL(
      "hack/bicep-types-radius/generated/radius/radius.core/2025-08-01-preview/types.json",
      root,
    ),
  );
  const resolve = (reference) => types[Number(reference.$ref.slice(2))];
  const resource = (name) => {
    const found = types.find(
      (type) =>
        type.$type === "ResourceType" &&
        type.name === `Radius.Core/${name}@2025-08-01-preview`,
    );
    assert.ok(found, name);
    return found;
  };
  for (const name of [
    "applications",
    "environments",
    "recipePacks",
    "bicepSettings",
    "terraformSettings",
  ]) {
    resource(name);
  }
  assert.equal(resource("roleDefinitions").readableScopes, 0);
  assert.equal(resource("roleDefinitions").writableScopes, 0);
  assert.equal(resource("roleAssignments").readableScopes, 0);
  assert.equal(resource("roleAssignments").writableScopes, 0);
  const body = (name) => resolve(resource(name).body);
  for (const name of ["roleDefinitions", "roleAssignments"]) {
    assert.ok(body(name).properties.scope, name);
  }
  assert.equal(body("roleAssignments").properties.name.flags & 8, 0);
  const properties = (name) =>
    resolve(body(name).properties.properties.type).properties;
  for (const key of ["scope", "managedBy", "createdBy"]) {
    assert.equal(properties("roleAssignments")[key].flags, 2, key);
  }
  assert.equal(properties("roleDefinitions").roleType.flags, 2);
  assert.deepEqual(
    resolve(body("authorizationSettings").properties.name.type),
    {
      $type: "StringLiteralType",
      value: "default",
    },
  );
  assert.equal(
    types.some(
      (type) =>
        type.$type === "ResourceType" &&
        /\/(accessChanges|permissions)@/.test(type.name),
    ),
    false,
  );
  const index = json(
    new URL("hack/bicep-types-radius/generated/index.json", root),
  );
  for (const name of [
    "roleDefinitions",
    "roleAssignments",
    "authorizationSettings",
  ]) {
    assert.ok(index.resources[`Radius.Core/${name}@2025-08-01-preview`]);
  }
});

test("generated Swagger preserves scope routes, read-only fields, and operation boundaries", () => {
  const spec = json(
    new URL(
      "swagger/specification/authorization/resource-manager/Radius.Core/preview/2025-08-01-preview/openapi.json",
      root,
    ),
  );
  assert.equal(spec.info.version, "2025-08-01-preview");
  const prefix = "/providers/Radius.Core";
  const methods = (path) =>
    Object.keys(spec.paths[path])
      .filter((key) => ["get", "put", "post", "patch", "delete"].includes(key))
      .sort();
  for (const scope of ["", "/{scope}"]) {
    for (const [type, key] of [
      ["roleDefinitions", "roleDefinitionName"],
      ["roleAssignments", "roleAssignmentName"],
    ]) {
      assert.deepEqual(methods(`${scope}${prefix}/${type}`), ["get"]);
      assert.deepEqual(methods(`${scope}${prefix}/${type}/{${key}}`), [
        "delete",
        "get",
        "put",
      ]);
    }
    assert.deepEqual(methods(`${scope}${prefix}/checkAccess`), ["post"]);
    assert.deepEqual(methods(`${scope}${prefix}/explainAccess`), ["post"]);
    assert.deepEqual(methods(`${scope}${prefix}/effectiveAccess`), ["get"]);
  }
  assert.deepEqual(methods(`${prefix}/authorizationSettings/default`), [
    "get",
    "put",
  ]);
  assert.deepEqual(methods(`${prefix}/permissions`), ["get"]);
  assert.deepEqual(methods(`${prefix}/accessChanges`), ["get"]);
  assert.deepEqual(methods(`${prefix}/accessChanges/{accessChangeName}`), [
    "get",
  ]);
  assert.deepEqual(methods(`${prefix}/whoAmI`), ["get"]);
  assert.equal(
    Object.keys(spec.paths).some((path) => /internal|preflight/.test(path)),
    false,
  );

  const definitions = Object.values(spec.definitions);
  const properties = (required) => {
    const model = definitions.find((definition) =>
      required.every((key) => Object.hasOwn(definition.properties ?? {}, key)),
    );
    assert.ok(model, `missing properties: ${required.join(", ")}`);
    return model.properties;
  };
  const role = properties(["displayName", "roleType", "permissions"]);
  assert.equal(role.roleType.readOnly, true);
  const assignment = properties([
    "roleDefinitionId",
    "principal",
    "scope",
    "managedBy",
    "createdBy",
  ]);
  for (const key of ["scope", "managedBy", "createdBy"]) {
    assert.equal(assignment[key].readOnly, true, key);
  }
  assert.deepEqual(assignment.roleDefinitionId["x-ms-mutability"], [
    "read",
    "create",
  ]);
  const principal = properties(["type", "issuer", "subject"]);
  for (const key of ["type", "issuer", "subject"]) {
    assert.deepEqual(
      principal[key]["x-ms-mutability"],
      ["read", "create"],
      key,
    );
  }
  const mode = definitions.find((definition) =>
    definition.enum?.includes("DryRun"),
  );
  assert.deepEqual([...mode.enum].sort(), ["Disabled", "DryRun", "Enforce"]);

  const resolve = (parameter) =>
    parameter.$ref?.startsWith("#/parameters/")
      ? spec.parameters[parameter.$ref.split("/").at(-1)]
      : parameter;
  const scopedPut =
    spec.paths[`/{scope}${prefix}/roleAssignments/{roleAssignmentName}`].put;
  const scope = scopedPut.parameters
    .map(resolve)
    .find((parameter) => parameter.name === "scope");
  assert.equal(scope.in, "path");
  assert.equal(scope["x-ms-skip-url-encoding"], true);

  const applicationSpec = json(
    new URL(
      "swagger/specification/radius/resource-manager/Radius.Core/preview/2025-08-01-preview/openapi.json",
      root,
    ),
  );
  assert.equal(
    Object.keys(applicationSpec.paths).some((path) =>
      /roleDefinitions|roleAssignments|authorizationSettings|accessChanges|checkAccess|whoAmI/.test(
        path,
      ),
    ),
    false,
  );
});
