import radius as rad

param application string
param environment string

resource s3Extender 'Applications.Link/extenders@2022-03-15-privatepreview' = {
  name: 's3'
  properties: {
    environment: environment
    application: application
    recipe: {
      name: 's4'
    }
  }
}

// resource container 'Applications.Core/containers@2022-03-15-privatepreview' = {
//   name: 'mycontainer'
//   properties: {
//     application: application
//     container: {
//       image: '*****'
//       env: {
//         BUCKETNAME: s3Extender.properties.bucketName
//       }
//     }
//     connections: {
//       s3: {
//         source: s3Extender.id
//       }
//     }
//   }
// }
