param magpieimage string = 'radiusdev.azurecr.io/magpiego:latest'

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'kubernetes-mechanics-redeploy-withtwoseparateresource'
}

resource b 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'b'
  properties: {
    container: {
      image: magpieimage
    }
  }
}
