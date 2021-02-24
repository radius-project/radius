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
          setEnv: {
            CONNECTIONSTRINGS_DB: 'connectionString'
          }
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