resource app 'radius.dev/Application@v1alpha3' = {
  name: 'trafficsplit'

  resource httpbinv1 'Container@v1alpha3' = {
    name: 'httpbin'

    properties: {
       container: {
        image: 'tommyniu.azurecr.io/httpbin:latest'
        ports: {
          http: {
            containerPort: 14001
            provides: httpbinv1route.id
          }
        }
        env: {
          'XHTTPBIN_POD':'httpbin-v1'
        }
       }
    }
  }

  resource httpbinv2 'Container@v1alpha3' = {
    name: 'httpbinv2'

    properties: {
       container: {
        image: 'tommyniu.azurecr.io/httpbin:latest'
        ports: {
          http: {
            containerPort: 14001
            provides: httpbinv2route.id
          }
        }
        env: {
          'XHTTPBIN_POD':'httpbin-v2'
        }
       }
    }
  }

  resource httpbinv2route 'HttpRoute@v1alpha3' = {
    name: 'httpbinroute-v2'
  }

  resource httpbinv1route 'HttpRoute@v1alpha3' = {
    name: 'httpbinroute-v1'
  }


  resource httpbin 'HttpRoute@v1alpha3' = {
    name:'httpbin'
    properties: {
      // targetport:14001
      routes: [
        {
          destination: httpbinv1route.id
          weight: 50
        }
        {
          destination:httpbinv2route.id
          weight:50
        }
      ]
    }
  }

}

resource curlapp 'radius.dev/Application@v1alpha3'= {
  name: 'curl'

  resource curl 'Container@v1alpha3' = {
      name:'curl'
      properties: {
        container: {
          image: 'tommyniu.azurecr.io/curl:latest'
        }
      }
    }
}
