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
import { Dictionary, filter, keys, orderBy } from 'lodash';
import { ArrayType, BuiltInType, DiscriminatedObjectType, getBuiltInTypeKindLabel, getObjectPropertyFlagsLabels, getScopeTypeLabels, ObjectProperty, ObjectType, ResourceFunctionType, ResourceType, StringLiteralType, TypeBase, TypeBaseKind, TypeReference, UnionType } from '../types';

export function writeTableMarkdown(provider: string, apiVersion: string, resourceTypes: ResourceType[], types: TypeBase[]) {
  let output = '';

  function getTypeName(types: TypeBase[], typeReference: TypeReference): string {
    const type = types[typeReference.Index];
    switch (type.Type) {
      case TypeBaseKind.BuiltInType:
        return getBuiltInTypeKindLabel((type as BuiltInType).Kind).toLowerCase();
      case TypeBaseKind.ObjectType:
        return generateAnchorLink((type as ObjectType).Name);
      case TypeBaseKind.ArrayType:
        return `${getTypeName(types, (type as ArrayType).ItemType)}[]`;
      case TypeBaseKind.ResourceType:
        return (type as ResourceType).Name;
      case TypeBaseKind.ResourceFunctionType: {
        const functionType = type as ResourceFunctionType;
        return `${functionType.Name} (${functionType.ResourceType}@${functionType.ApiVersion})`;
      }
      case TypeBaseKind.UnionType: {
        const elements = (type as UnionType).Elements.map(x => getTypeName(types, x));
        return elements.sort().join(' | ');
      }
      case TypeBaseKind.StringLiteralType:
        return `'${(type as StringLiteralType).Value}'`;
      case TypeBaseKind.DiscriminatedObjectType:
        return generateAnchorLink((type as DiscriminatedObjectType).Name);
      default:
        throw `Unrecognized type`;
    }
  }

  function generateAnchorLink(name: string) {
    return `[${name}](#${name.replace(/[^a-zA-Z0-9-]/g, '').toLowerCase()})`;
  }

  function writeTypeProperty(types: TypeBase[], name: string, property: ObjectProperty) {
    const flagsString = property.Flags ? `${getObjectPropertyFlagsLabels(property.Flags).join(', ')}` : '';
    const descriptionString = property.Description ? property.Description : '';
    writeTableEntry(name, getTypeName(types, property.Type), flagsString, descriptionString);
  }

  function writeTableHeading(){
    output += `| Property | Type | Description |\n`;
    output += `|----------|------|-------------|\n`;
  }

  function writeTableEntry(name: string, type: string, flags: string, description: string){
    const flagString = flags ? `<br />_(${flags})_ ` : '';
    output += `| **${name}** | ${type} | ${description} ${flagString}|\n`;
  }

  function writeHeading(nesting: number, message: string) {
    output += `${'#'.repeat(nesting)} ${message}`;
    writeNewLine();
  }

  function writeBullet(key: string, value: string) {
    output += `* **${key}**`;
    if (value != "") {
      output += `: ${value}`;
    }
    writeNewLine();
  }

  function writeNewLine() {
    output += '\n';
  }

  function findTypesToWrite(types: TypeBase[], typesToWrite: TypeBase[], typeReference: TypeReference) {
    function processTypeLinks(typeReference: TypeReference, skipParent: boolean) {
      // this is needed to avoid circular type references causing stack overflows
      if (typesToWrite.indexOf(types[typeReference.Index]) === -1) {
        if (!skipParent) {
          typesToWrite.push(types[typeReference.Index]);
        }

        findTypesToWrite(types, typesToWrite, typeReference);
      }
    }

    const type = types[typeReference.Index];
    switch (type.Type) {
      case TypeBaseKind.ArrayType: {
        const arrayType = type as ArrayType;
        processTypeLinks(arrayType.ItemType, false);

        return;
      }
      case TypeBaseKind.ObjectType: {
        const objectType = type as ObjectType;

        for (const key of sortedKeys(objectType.Properties)) {
          processTypeLinks(objectType.Properties[key].Type, false);
        }

        if (objectType.AdditionalProperties) {
          processTypeLinks(objectType.AdditionalProperties, false);
        }

        return;
      }
      case TypeBaseKind.DiscriminatedObjectType: {
        const discriminatedObjectType = type as DiscriminatedObjectType;

        for (const key of sortedKeys(discriminatedObjectType.BaseProperties)) {
          processTypeLinks(discriminatedObjectType.BaseProperties[key].Type, false);
        }

        for (const key of sortedKeys(discriminatedObjectType.Elements)) {
          const element = discriminatedObjectType.Elements[key];
          // Don't display discriminated object elements as individual types
          processTypeLinks(element, true);
        }

        return;
      }
    }
  }

  function sortedKeys<T>(dictionary: Dictionary<T>) {
    return orderBy(keys(dictionary), k => k.toLowerCase(), 'asc');
  }

  function writeComplexType(types: TypeBase[], type: TypeBase, nesting: number, includeHeader: boolean) {
    switch (type.Type) {
      case TypeBaseKind.ResourceType: {
        const resourceType = type as ResourceType;
        writeHeading(nesting, `Top-Level Resource`);
        // temporarily removing scope as it's not applicable
        // writeBullet("Valid Scope(s)", `${getScopeTypeLabels(resourceType.ScopeType).join(', ') || 'Unknown'}`);
        writeComplexType(types, types[resourceType.Body.Index], nesting, false);

        return;
      }
      case TypeBaseKind.ResourceFunctionType: {
        const resourceFunctionType = type as ResourceFunctionType;
        writeHeading(nesting, `Function ${resourceFunctionType.Name} (${resourceFunctionType.ResourceType}@${resourceFunctionType.ApiVersion})`);
        writeNewLine();
        writeBullet("Resource", resourceFunctionType.ResourceType);
        writeBullet("ApiVersion", resourceFunctionType.ApiVersion);
        if (resourceFunctionType.Input) {
          writeBullet("Input", getTypeName(types, resourceFunctionType.Input));
        }
        writeBullet("Output", getTypeName(types, resourceFunctionType.Output));

        writeNewLine();
        return;
      }
      case TypeBaseKind.ObjectType: {
        const objectType = type as ObjectType;
        if (includeHeader) {
          writeHeading(nesting, objectType.Name);
        }

        writeNewLine();
        writeHeading(nesting + 1, "Properties");
        writeNewLine();

        if (Object.keys(objectType.Properties).length === 0) {
          writeBullet("none", "");
          writeNewLine();
        }
        else {
          writeTableHeading();
          for (const key of sortedKeys(objectType.Properties)) {
            writeTypeProperty(types, key, objectType.Properties[key]);
          }
        }

        if (objectType.AdditionalProperties) {
          writeHeading(nesting + 1, "Additional Properties");
          writeNewLine();
          writeBullet("Additional Properties Type", getTypeName(types, objectType.AdditionalProperties));
        }

        writeNewLine();
        return;
      }
      case TypeBaseKind.DiscriminatedObjectType: {
        const discriminatedObjectType = type as DiscriminatedObjectType;
        if (includeHeader) {
          writeHeading(nesting, discriminatedObjectType.Name);
          writeNewLine();
        }

        writeBullet("Discriminator", discriminatedObjectType.Discriminator);
        writeNewLine();

        writeHeading(nesting + 1, "Base Properties");
        writeNewLine();
        if (Object.keys(discriminatedObjectType.BaseProperties).length === 0) {
          writeBullet("none", "");
          writeNewLine();
        }
        else {
          writeTableHeading();
          for (const propertyName of sortedKeys(discriminatedObjectType.BaseProperties)) {
            writeTypeProperty(types, propertyName, discriminatedObjectType.BaseProperties[propertyName]);
          }
        }

        writeNewLine();
        
        for (const key of sortedKeys(discriminatedObjectType.Elements)) {
          const element = discriminatedObjectType.Elements[key];
          writeComplexType(types, types[element.Index], nesting + 1, true);
        }

        writeNewLine();
        return;
      }
    }
  }

  function generateMarkdown(provider: string, apiVersion: string, types: TypeBase[]) {

    const resourceFunctionTypes = orderBy(types.filter(t => t instanceof ResourceFunctionType) as ResourceFunctionType[], x => x.Name.split('@')[0].toLowerCase());    
    const filteredFunctionTypes = resourceFunctionTypes.filter(x => resourceTypes.some(y => x.ResourceType.toLowerCase() === y.Name.split('@')[0].toLowerCase()));
    const typesToWrite: TypeBase[] = [...resourceTypes, ...filteredFunctionTypes];

    for (const resourceType of resourceTypes) {
      findTypesToWrite(types, typesToWrite, resourceType.Body);
    }

    for (const resourceFunctionType of filteredFunctionTypes) {
      if (resourceFunctionType.Input) {
        typesToWrite.push(types[resourceFunctionType.Input.Index]);
        findTypesToWrite(types, typesToWrite, resourceFunctionType.Input);
      }
      typesToWrite.push(types[resourceFunctionType.Output.Index]);
      findTypesToWrite(types, typesToWrite, resourceFunctionType.Output);
    }

    for (const type of typesToWrite) {
      writeComplexType(types, type, 3, true);
    }

    return output;
  }

  return generateMarkdown(provider, apiVersion, types);
}