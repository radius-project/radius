// This example is hypothetical for now
resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'cosmos-container'
  
  resource webapp 'Components' = {
    name: 'webapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'rynowak/my-cool-webapp:latest'
        }
      }
      dependsOn: [
        {
          name: 'db'
          kind: 'database'
          set: {
            store: 'my-keyvault'
            kind: 'azure.com/KeyVault'
            values: {
              CONNECTIONSTRINGS_DB: 'connectionString'
            }
          }
        }

        // Document that the application has a dependency on a keyvault
        {
          name: 'my-keyvault'
          kind: 'azure.com/KeyVault'
        }
      ]
    }
  }

  resource db 'Components' = {
    name: 'db'
    kind: 'azure.com/CosmosDocumentDb@v1alpha1'
    properties: {
      config: {
        // more config here
      }
    }
  }
}

// Imagine that this is configured elsewhere by someone else
// scope keys 'azure.com/KeyValue@v1alpha1' = {
//   name: 'my-keyvault'
//   properties: {
//     config: {
//       // Configuration for keyvault - or a link to a resource goes here
//     }
//   }
// }
