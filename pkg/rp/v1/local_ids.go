// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v1

import (
	"encoding/base64"
	"encoding/binary"
	"hash/fnv"
)

// About LocalIDs
//
// The LocalID concept exists to give each 'output resource' a *logical* name that is a unique and stable per-Radius-resource.
// This means that given the same inputs to a renderer, the outputs will contain the same set of LocalIDs. The LocalIDs are
// *not* randomly generated, they must be stable and predictable given the same input.
//
// Physical names (think resource ID) *must* be decoupled from our business logic, because they almost always have to be generated
// with a non-idempotent process due to uniqueness requirements. Consider the physical name of a resource to be a side-effect of creating
// it.
//
// Since we can't use physical names, LocalIDs exist to give us an identifier we *can* use.
//
// This is needed for a few reasons:
//
// This allows renderers to refer to and create dependencies between resources without *knowing* the physical names of those resources.
// The capability to refer to logical names is critical when multiple resources have a contract with each other (eg. a KeyVault)
// creates a KeyVault resource, and the container needs a role assignment to access it.)
//
// This allows diffing of expected state vs actual state. We can tell when a physical resource has disappeared. We can tell when a logical
// resource *should* disappear. This allows the renders a degree of control when the user-provided definition of a Radius resource changes.

// Represents local IDs used for output resources
const (
	LocalIDAzureCosmosAccount           = "AzureCosmosAccount"
	LocalIDAzureCosmosDBMongo           = "AzureCosmosDBMongo"
	LocalIDAzureCosmosDBSQL             = "AzureCosmosDBSQL"
	LocalIDAzureFileShare               = "AzureFileShare"
	LocalIDAzureFileShareStorageAccount = "AzureFileShareStorageAccount"
	LocalIDAzureRedis                   = "AzureRedis"
	LocalIDAzureServiceBusNamespace     = "AzureServiceBusNamespace"
	LocalIDAzureServiceBusTopic         = "AzureServiceBusTopic"
	LocalIDAzureServiceBusQueue         = "AzureServiceBusQueue"
	LocalIDAzureSqlServer               = "AzureSqlServer"
	LocalIDAzureSqlServerDatabase       = "AzureSqlServerDatabase"
	LocalIDExtender                     = "Extender"
	LocalIDDaprStateStoreAzureStorage   = "DaprStateStoreAzureStorage"
	LocalIDDaprStateStoreComponent      = "DaprStateStoreComponent"
	LocalIDDaprStateStoreSQLServer      = "DaprStateStoreSQLServer"
	LocalIDDaprComponent                = "DaprComponent"
	LocalIDDeployment                   = "Deployment"
	LocalIDGateway                      = "Gateway"
	LocalIDHttpRoute                    = "HttpRoute"
	LocalIDKeyVault                     = "KeyVault"
	LocalIDRabbitMQDeployment           = "KubernetesRabbitMQDeployment"
	LocalIDRabbitMQSecret               = "KubernetesRabbitMQSecret"
	LocalIDRabbitMQService              = "KubernetesRabbitMQService"
	LocalIDRedisDeployment              = "KubernetesRedisDeployment"
	LocalIDRedisService                 = "KubernetesRedisService"
	LocalIDScrapedSecret                = "KubernetesScrapedSecret"
	LocalIDSecret                       = "Secret"
	LocalIDSecretProviderClass          = "SecretProviderClass"
	LocalIDServiceAccount               = "ServiceAccount"
	LocalIDKubernetesRole               = "KubernetesRole"
	LocalIDKubernetesRoleBinding        = "KubernetesRoleBinding"
	LocalIDService                      = "Service"
	LocalIDStatefulSet                  = "StatefulSet"
	LocalIDUserAssignedManagedIdentity  = "UserAssignedManagedIdentity"
	LocalIDFederatedIdentity            = "FederatedIdentity"

	// Obsolete when we remove AppModelV1
	LocalIDRoleAssignmentKVKeys         = "RoleAssignment-KVKeys"
	LocalIDRoleAssignmentKVSecretsCerts = "RoleAssignment-KVSecretsCerts"
	LocalIDKeyVaultSecret               = "KeyVaultSecret"
)

// Most LocalIDs are a 1:1 mapping with Radius resource.
//
// This is a little tricky for role assignments because we need to key them on the resource ID
// of the target resource X the role being assigned. For example if the user switches their keyvault 'a'
// for a different instance 'b' we want to delete the original role assignments and create new ones.
func GenerateLocalIDForRoleAssignment(id string, roleName string) string {
	base := "RoleAssignment-"

	// The technique here uses a stable hashing algorithm with 32 bits of entropy. These values
	// only need to be unique within a *single* Radius resource.
	h := fnv.New32a()
	_, _ = h.Write([]byte(id))
	_, _ = h.Write([]byte(roleName))

	hash := [4]byte{}
	binary.BigEndian.PutUint32(hash[:], h.Sum32())
	return base + base64.StdEncoding.EncodeToString(hash[:])
}
