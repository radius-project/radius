// This example is hypothetical for now
application app = {
  name: 'cosmos-container'
  
  instance webapp 'radius.dev/Container@v1alpha1' = {
    name: 'webapp'
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

  instance db 'azure.com/CosmosDocumentDb@v1alpha1' = {
    name: 'db'
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