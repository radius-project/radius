import radius as radius

resource env 'Applications.Core/environments@2022-03-15-privatepreview' = {
  name: 'corerp-kubemetadata-env'
  location: 'global'
   properties: {
   compute: {
     kind: 'kubernetes'
     namespace: 'my-ns'
   } 
   extensions: [
    {
       kind: 'kubernetesMetadata'
       annotations: {
        'user.env.ann.1' : 'user.env.ann.val.1'
        'user.env.ann.2' : 'user.env.ann.val.2'
       }
       labels: {
        'user.env.lbl.1' : 'user.env.lbl.val.1'
        'user.env.lbl.2' : 'user.env.lbl.val.2'
       }      
    }
   ]
  }
}

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-kubemetadata-app' 
  location: 'global'
  properties: {
     environment: env.id
     extensions: [
      {
        kind: 'kubernetesMetadata'
        annotations:{
          'user.app.ann.1' : 'user.app.ann.val.1'
          'user.app.ann.2' : 'user.app.ann.val.2'
        }
        labels:{
          'user.app.lbl.1' : 'user.app.lbl.val.1'
          'user.app.lbl.2' : 'user.app.lbl.val.2'
        }
      }
     ]
   }
    
}
