extension radius
extension testresources
// extension hack
// param registry string

// param version string

@description('The URL of the server hosting test Terraform modules.')
param moduleServer string

@description('PostgreSQL password')
@secure()
param password string = newGuid()

resource udttoudtenv 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'udttoudtenv'
  location: 'global'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'udttoudtenv'
    }
    recipes: {
      'Test.Resources/udtChild': {
        default: {
          templateKind: 'terraform'
          templatePath: '${moduleServer}/child-udt.zip'
        }
      }
      'Test.Resources/udtParent': {
        default: {
          templateKind: 'terraform'
          templatePath: '${moduleServer}/parent-udt.zip'
        }
      }
    }
  }
}

resource udttoudtapp 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'udttoudtapp'
  location: 'global'
  properties: {
    environment: udttoudtenv.id
    extensions: [
      {
        kind: 'kubernetesNamespace'
        namespace: 'udttoudtapp'
      }
    ]
  }
}


resource udtparent 'Test.Resources/udtParent@2023-10-01-preview' = {
    name: 'udtparent'
    properties: {
      environment: udttoudtenv.id
      application: udttoudtapp.id
      size: 'S'
      connections: {
        databaseresource: {
          source: udtchild.id
        }
      }
    }     
}


resource udtchild 'Test.Resources/udtChild@2023-10-01-preview' = {
  name: 'udtchild'
  location: 'global'
  properties: {
    environment: udttoudtenv.id
    application: udttoudtapp.id
    size: 'S'
  }
}
