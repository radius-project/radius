---
type: docs
title: "Radius Volume"
linkTitle: "Volume"
description: "Learn about the Radius Persistent Volume"
weight: 100
---

`Volume` provides an abstraction for a persistent volume to be referenced and mounted by a [ContainerComponent]({{< ref container >}}). Persistent volumes have lifecycles that are separate from the container. ContainerComponents "attach" to another resource which contains the volume.

### Azure File Share
A persistent volume of kind azure.com.fileshare mounts an Azure File Share to a container. See [Azure File Share](https://docs.microsoft.com/en-us/azure/storage/files/storage-files-introduction)

### Azure Key Vault
A persistent volume of kind azure.com.keyvault mounts an Azure KeyVault to a container using Azure KeyVault CSI driver. See [Azure Key Vault CSI Driver](https://azure.github.io/secrets-store-csi-driver-provider-azure/demos/standard-walkthrough/) for additional details on the CSI driver. Note that the access policy for the Azure Key Vault should be set to Azure role-based access control.

## Supported resources

| Resource | kind |
|-----------|------|
| [Azure File Share](#Azure-File-Share) | `'azure.com.fileshare'`
| [Azure Key Vault](#Azure-Key-Vault) | `'azure.com.keyvault'`

## Secrets

| Field | Description | Required | Example |
| ------|:-----:|:---:|:--------:|:--------|
| name | secret name in Azure Key Vault | true | `'mysecret'`
| version | specific secret version. Default is latest | false | `'1234'`
| encoding | encoding format 'utf-8', 'hex', 'base64'. Default is 'utf-8' | false | `'bas64'`
| alias | file name created on the disk. Same as objectname if not specified | false | `'my-secret'`

## Keys

| Field | Description | Required | Example |
| ------|:-----:|:---:|:--------:|:--------|
| name | key name in Azure Key Vault | true | `'mykey'`
| version | specific key version. Default is latest | false | `'1234'`
| alias | file name created on the disk. Same as objectname if not specified | false | `'my-key'`

## Certificates

| Field | Description | Required | Example |
| ------|:-----:|:---:|:--------:|:--------|
| name | certificate name in Azure Key Vault | true | `'mycert'`
| value | value to download from Azure Key Vault 'privatekey', 'publickey' or 'certificate' | true | `'certificate'`
| version | specific certificate version. Default is latest | false | `'1234'`
| encoding | encoding format 'utf-8', 'hex', 'base64'. Default is 'utf-8' and this field can be specificed only when value is 'privatekey' | false | `'bas64'`
| alias | file name created on the disk. Same as objectname if not specified | false | `'my-cert'`
| format | certificate format 'pfx', 'pem'. Default is 'pfx' | false | `'my-cert'`

## Component format

{{< rad file="snippets/volume.bicep" embed=true marker="//SAMPLE" >}}

### Properties

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| kind | y | The kind of persistent volume. See [supported volumes](#supported-resources). | `'azure.com.fileshare'`
| managed | n | If set to 'true', Radius will manage the lifecycle and configuration of the underlying resource. Defaults to 'false'. | `'true'`, `'false'`
| resource | n | Resource ID for the existing resource. Used for an unmanaged resource. | `'share.id'`, `'/subscriptions/<subscription>/resourceGroups/<rg/providers/Microsoft.Storage/storageAccounts/<storageAccountName>/fileServices/default/shares/<fileshareName>'`
| secrets | n | Map specify secret object name and secret properties. See [secret properties] (#secrets) | <code>mysecret: {<br>name: 'mysecret'{<br>encoding: 'utf-8{<br>}</code>
| keys | n | Map specify key object name and key properties. See [key properties] (#keys) | <code>mykey: {<br>name: 'mykey'<br>}</code>
| certificates | n | Map specify certificate object name and [certificate properties]. See [#certificates] (#certificate properties) | <code>mycert: {<br>name: 'mycert'<br>value: 'certificate'<br>}</code>
