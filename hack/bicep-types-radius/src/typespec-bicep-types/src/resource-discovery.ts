// ------------------------------------------------------------
// Copyright 2023 The Radius Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// ------------------------------------------------------------.

import type { Model, Program } from "@typespec/compiler";
import { getAllHttpServices, type HttpOperation } from "@typespec/http";
import { getArmResources } from "@azure-tools/typespec-azure-resource-manager";
import { getVersions } from "@typespec/versioning";
import { ScopeType } from "./bicep.js";
import { armResourceKindToScopeType } from "./scopes.js";

/** A resource action (e.g. `listSecrets`) surfaced as a Bicep resource function. */
export interface DiscoveredAction {
  /** The action name, used as the Bicep resource function name. */
  name: string;
  /** The HTTP operation backing the action (carries the request/response bodies). */
  httpOperation: HttpOperation;
}

/**
 * A resource type discovered from the TypeSpec program, normalized to the shape
 * the Bicep type translator needs. This replaces the OpenAPI path-parsing in the
 * AutoRest extension's `resources.ts`: the ARM library exposes the provider
 * namespace, collection name, key parameter, scope kind, and the backing model
 * as declared metadata, so nothing has to be reverse-engineered from URLs.
 */
export interface DiscoveredResource {
  /** Fully qualified type, e.g. `Applications.Messaging/rabbitMQQueues`. */
  fullyQualifiedType: string;
  /** API version, e.g. `2023-10-01-preview`. */
  apiVersion: string;
  /** Bicep resource type name, e.g. `Applications.Messaging/rabbitMQQueues@2023-10-01-preview`. */
  resourceTypeName: string;
  /** Provider namespace, e.g. `Applications.Messaging`. */
  namespace: string;
  /** The TypeSpec model backing the resource body. */
  bodyModel: Model;
  /** The resource name parameter, when one is declared. */
  keyName?: string;
  readableScopes: ScopeType;
  writableScopes: ScopeType;
  /** Resource actions surfaced as Bicep resource functions (e.g. `listSecrets`). */
  actions: DiscoveredAction[];
}

/**
 * Discovers every ARM resource in the program and normalizes it for the Bicep
 * type translator.
 */
export function discoverResources(program: Program): DiscoveredResource[] {
  const apiVersion = resolveApiVersion(program);
  const discovered: DiscoveredResource[] = [];

  for (const resource of getArmResources(program)) {
    if (!resource.collectionName) {
      continue;
    }

    const fullyQualifiedType = `${resource.armProviderNamespace}/${resource.collectionName}`;
    const scope = armResourceKindToScopeType(resource.kind);

    // ARM groups POST actions under operations.actions (GET lists live under
    // operations.lists), which is exactly the set the AutoRest path surfaced as
    // resource functions.
    const actions: DiscoveredAction[] = Object.values(
      resource.operations.actions,
    ).map((operation) => ({
      name: operation.name,
      httpOperation: operation.httpOperation,
    }));

    discovered.push({
      fullyQualifiedType,
      apiVersion,
      resourceTypeName: `${fullyQualifiedType}@${apiVersion}`,
      namespace: resource.armProviderNamespace,
      bodyModel: resource.typespecType,
      keyName: resource.keyName,
      readableScopes: scope,
      writableScopes: scope,
      actions,
    });
  }

  return discovered;
}

/**
 * Resolves the service API version from the program's `@versioned` metadata,
 * taking the latest declared version. Radius specs declare a single preview
 * version, so this is the value embedded in the resource type names.
 */
function resolveApiVersion(program: Program): string {
  const [services] = getAllHttpServices(program);

  for (const service of services) {
    const [, versionMap] = getVersions(program, service.namespace);
    const versions = versionMap?.getVersions() ?? [];
    if (versions.length > 0) {
      return versions[versions.length - 1].value;
    }
  }

  return "unknown";
}
