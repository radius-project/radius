extension radius

@description('JSON-encoded list of subnet IDs for the DB subnet group')
param subnetIds string

@description('JSON-encoded list of VPC security group IDs')
param vpcSecurityGroupIds string

@description('RDS instance class')
param instanceClass string = 'db.t3.micro'

@description('Allocated storage in GB')
param allocatedStorage int = 20

@description('Kubernetes namespace for the environment')
param namespace string = 'default'

resource recipepack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'aws-mysql-pack'
  location: 'global'
  properties: {
    recipes: {
      'Applications.Datastores/sqlDatabases': {
        kind: 'terraform'
        location: 'terraform-aws-modules/rds/aws'
        parameters: {
          // RDS instance config
          identifier: '{{context.resource.name}}'
          engine: 'mysql'
          engine_version: '8.4'
          family: 'mysql8.4'
          major_engine_version: '8.4'
          instance_class: instanceClass
          allocated_storage: allocatedStorage
          storage_type: 'gp3'

          // Database config
          db_name: '{{context.resource.properties.database}}'
          username: '{{context.resource.properties.username}}'
          port: 3306

          // Networking
          vpc_security_group_ids: vpcSecurityGroupIds
          create_db_subnet_group: true
          subnet_ids: subnetIds

          // Operational
          skip_final_snapshot: true
          apply_immediately: true
        }
        outputs: {
          host: 'db_instance_address'
          port: 'db_instance_port'
          database: 'db_instance_name'
        }
      }
    }
  }
}

resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'aws-mysql-env'
  location: 'global'
  properties: {
    recipePacks: [
      recipepack.id
    ]
    providers: {
      kubernetes: {
        namespace: namespace
      }
    }
  }
}
