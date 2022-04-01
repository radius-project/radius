package common

// SystemDataProperties is the readonly metadata pertaining to creation and last modification of the resource.
// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/common-api-contracts.md#system-metadata-for-all-azure-resources
type SystemDataProperties struct {
	// CreatedBy is a string identifier for the identity that created the resource.
	CreatedBy string `json:"createdBy,omitempty"`
	// CreatedByType is the type of identity that created the resource: user, application, managedIdentity.
	CreatedByType string `json:"createdByType,omitempty"`
	// CreatedAt is the timestamp of resource creation (UTC).
	CreatedAt string `json:"createdAt,omitempty"`
	// LastModifiedBy is a string identifier for the identity that last modified the resource.
	LastModifiedBy string `json:"lastModifiedBy,omitempty"`
	// LastModifiedBy is the type of identity that last modified the resource: user, application, managedIdentity
	LastModifiedByType string `json:"lastModifiedByType,omitempty"`
	// LastModifiedBy is the timestamp of resource last modification (UTC).
	LastModifiedAt string `json:"lastModifiedAt,omitempty"`
}
