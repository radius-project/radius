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
          setEnv: {
            CONNECTIONSTRINGS_DB: 'connectionString'
          }
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

// Imagine this is is another file, managed by someone else
// scope plan 'azure.com/WebsiteHostingPlan' = {
//   name: 'default'
//   properties: {
//     sku: {
//       name: 'F1'
//       capacity: 1
//     }
//   }
// }
