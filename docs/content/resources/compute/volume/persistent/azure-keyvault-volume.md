---
type: docs
title: "Azure KeyVault"
linkTitle: "Azure KeyVault"
description: "Learn about the Radius Persistent Azure KeyVault Volume"
weight: 200
---

Radius supports mounting an Azure KeyVault as a persistent volume to the container using the Azure KeyVault CSI Driver. See [Azure Key Vault CSI Driver](https://azure.github.io/secrets-store-csi-driver-provider-azure/demos/standard-walkthrough/) for additional details on the CSI driver. Note that for the Azure Key Vault that is mounted as a CSI volume, the **access policy** should be set to **Azure role-based access control**.

## Component format

{{< rad file="snippets/volume-keyvault-csi.bicep" embed=true marker="//SAMPLE" >}}

### Properties

You need to specify the properties below on the volume resource to which the container attaches:-

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| kind | y | The kind of persistent volume. Should be 'azure.com.keyvault' for Azure Key Vault persistent volumes | `'azure.com.keyvault'`
| managed | y | Only unmanaged Azure KeyVault is supported for mounting using CSI Driver. | `'false'`
| resource | n | Resource ID for the Azure KeyVault resource. | `'kv.id'`, `'/subscriptions/<subscription>/resourceGroups/<rg/providers/Microsoft.KeyVault/vaults/<keyvaultname>'`
| secrets | n | Map specify secret object name and secret properties. See [secret properties](#secrets) | <code>mysecret: {<br>name: 'mysecret'{<br>encoding: 'utf-8{<br>}</code>
| keys | n | Map specify key object name and key properties. See [key properties](#keys) | <code>mykey: {<br>name: 'mykey'<br>}</code>
| certificates | n | Map specify certificate object name and [certificate properties]. See [certificate properties](#certificate) | <code>mycert: {<br>name: 'mycert'<br>value: 'certificate'<br>}</code>

### Secrets

| Key  | Description | Required | Example |
|------|:--------:|-------------|---------|
| name | secret name in Azure Key Vault | true | `'mysecret'`
| version | specific secret version. Default is latest | false | `'1234'`
| encoding | encoding format 'utf-8', 'hex', 'base64'. Default is 'utf-8' | false | `'bas64'`
| alias | file name created on the disk. Same as objectname if not specified | false | `'my-secret'`

### Keys

| Key  | Description | Required | Example |
|------|:--------:|-------------|---------|
| name | key name in Azure Key Vault | true | `'mykey'`
| version | specific key version. Default is latest | false | `'1234'`
| alias | file name created on the disk. Same as objectname if not specified | false | `'my-key'`

### Certificates

| Key  | Description | Required | Example |
|------|:--------:|-------------|---------|
| name | certificate name in Azure Key Vault | true | `'mycert'`
| value | value to download from Azure Key Vault 'privatekey', 'publickey' or 'certificate' | true | `'certificate'`
| version | specific certificate version. Default is latest | false | `'1234'`
| encoding | encoding format 'utf-8', 'hex', 'base64'. Default is 'utf-8' and this field can be specificed only when value is 'privatekey' | false | `'bas64'`
| alias | file name created on the disk. Same as objectname if not specified | false | `'my-cert'`
| format | certificate format 'pfx', 'pem'. Default is 'pfx' | false | `'my-cert'`
