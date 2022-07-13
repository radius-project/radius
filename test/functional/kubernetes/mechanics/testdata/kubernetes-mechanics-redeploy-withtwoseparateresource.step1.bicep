param magpieimage string = 'radiusdev.azurecr.io/magpiego:latest' 

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'kubernetes-mechanics-redeploy-withtwoseparateresource'
}

resource a 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'a'
  properties: {
    container: {
      image: magpieimage
    }
  }
}
