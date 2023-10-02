import aws as aws

param creationTimestamp string
param dbSubnetGroupName string
param dbName string

resource subnetGroup 'AWS.RDS/DBSubnetGroup@default' = {
  alias: dbSubnetGroupName
  properties: {
    DBSubnetGroupDescription: dbSubnetGroupName
    SubnetIds: ['']
    Tags: [
      {
        Key: 'RadiusCreationTimestamp'
        Value: creationTimestamp
      }
    ]
  }
}

resource db 'AWS.RDS/DBInstance@default' = {
  alias: dbName
  properties: {
    DBInstanceIdentifier: dbName
    Engine: 'sqlserver-ex'
    EngineVersion: '15.00.4153.1.v1'
    DBInstanceClass: 'db.t3.small'
    AllocatedStorage: '20'
    MaxAllocatedStorage: 30
    StorageEncrypted: false
    MasterUsername: 'username'
    MasterUserPassword: 'password'
    Port: '1433'
    DBSubnetGroupName: dbSubnetGroupName
    DBSecurityGroups: []
    PreferredMaintenanceWindow: 'Mon:00:00-Mon:03:00'
    PreferredBackupWindow: '03:00-06:00'
    LicenseModel: 'license-included'
    Timezone: 'GMT Standard Time'
    CharacterSetName: 'Latin1_General_CI_AS'
    Tags: [
      {
        Key: 'RadiusCreationTimestamp'
        Value: creationTimestamp
      }
    ]
  }
}
