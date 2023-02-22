import aws as aws

param dbSubnetGroupName string = 'willsmith-rds-mssql-subnet-group-3'
resource subnetGroup 'AWS.RDS/DBSubnetGroup@default' = {
  alias: dbSubnetGroupName
  properties: {
    DBSubnetGroupDescription: dbSubnetGroupName
    SubnetIds: ['']
  }
}

param dbName string = 'willsmith-rds-mssql-3'
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
    Port: '1434'
    DBSubnetGroupName: dbSubnetGroupName
    DBSecurityGroups: []
    PreferredMaintenanceWindow: 'Mon:00:00-Mon:03:00'
    PreferredBackupWindow: '03:00-06:00'
    LicenseModel: 'license-included'
    Timezone: 'GMT Standard Time'
    CharacterSetName: 'Latin1_General_CI_AS'
  }
}
