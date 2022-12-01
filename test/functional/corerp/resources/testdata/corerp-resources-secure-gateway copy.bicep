import kubernetes as kubernetes {
  kubeConfig: ''
  namespace: 'default'
}
import radius as radius

@description('Specifies the location for resources.')
param location string = 'local'

@description('Specifies the environment for resources.')
param environment string

@description('Specifies the port for the container resource.')
param port int = 3000

@description('Specifies the image for the container resource.')
param magpieimage string

@description('Specifies the certificate for the container resource.')
param certificate string = '''
-----BEGIN CERTIFICATE-----
MIIECjCCAvICCQDN+eL0EhGeODANBgkqhkiG9w0BAQsFADCBxjELMAkGA1UEBhMC
VVMxEzARBgNVBAgMCkNhbGlmb3JuaWExETAPBgNVBAcMCFNhbiBKb3NlMRIwEAYD
VQQKDAlNaWNyb3NvZnQxDjAMBgNVBAsMBUFBQ1RPMUQwQgYDVQQDDDtodHRwcy1n
dHd5LmNvcmVycC1yZXNvdXJjZXMtZ2F0ZXdheXh4eC4yMC4xMDIuMjYuMTgzLm5p
cC5pbzElMCMGCSqGSIb3DQEJARYWbml0aHlhc3VAbWljcm9zb2Z0LmNvbTAeFw0y
MjExMjkxODQzMzZaFw0yMzExMjkxODQzMzZaMIHGMQswCQYDVQQGEwJVUzETMBEG
A1UECAwKQ2FsaWZvcm5pYTERMA8GA1UEBwwIU2FuIEpvc2UxEjAQBgNVBAoMCU1p
Y3Jvc29mdDEOMAwGA1UECwwFQUFDVE8xRDBCBgNVBAMMO2h0dHBzLWd0d3kuY29y
ZXJwLXJlc291cmNlcy1nYXRld2F5eHh4LjIwLjEwMi4yNi4xODMubmlwLmlvMSUw
IwYJKoZIhvcNAQkBFhZuaXRoeWFzdUBtaWNyb3NvZnQuY29tMIIBIjANBgkqhkiG
9w0BAQEFAAOCAQ8AMIIBCgKCAQEAvvhLs7vD/b4cbXxgPhmGrgIvNgGAFMBEvtf8
E15S0ZtRTiItW2BE+uzpqA9P40kr5reO9Cpq0kE5YSMdyNwFeOQ4x+ku4QpHJbeH
AL6Vumonu9WfxJxoXAwWMA9oyeEIt1p3Cy3pd2ubjMmhUgqAr46ze9v4hyWBp4O0
UorZt+lsAGbT47pMG4CIIVQwypq7PBOHZ/+SJzYAXkafcTNk27Ty1ls/X59H1jfz
4nIB97QLl8QufkDtzcWvhYvMlc8WMteT7H9jYuGVEWbOaN8YopdWGBwalikxuXxQ
LyK2lxdmQEYypKxkrSiuXfJU/s1l7sPtA4DGi9f1GTFz7bsCqwIDAQABMA0GCSqG
SIb3DQEBCwUAA4IBAQC9i3uMDuQYFmfK7KD0sKw05g0qb17zIUjoelxhIS0cva6F
At+8RibMxMFX5rkDYnivsdqYKXVCBxLCnaOejRNc1CdOu8hofoOVMlGNwMW3gfpl
tMo0/lyyS+N/51THAPd/HKKr7+4IIwaR9Fnf/pE5+WV3/NfzkAyfYOG8OHyoA5/O
nT4f7e6mVt8xpyY3PrpsRm+Wg/80COUEr6/4TcjsJRh1JTRFWxBp1b/upvNYtHrk
u20fmZjZf2T7UK+tADWqu2m3OdMnRg6sL3viKczXlFAfh65W1L0yVsFkwxGNXIRd
zyWDe2yEpdEaFF4atKlSVLUDiGyg7hxoY1uz/k/5
-----END CERTIFICATE-----
'''

param key string = '''
-----BEGIN PRIVATE KEY-----
MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQC++Euzu8P9vhxt
fGA+GYauAi82AYAUwES+1/wTXlLRm1FOIi1bYET67OmoD0/jSSvmt470KmrSQTlh
Ix3I3AV45DjH6S7hCkclt4cAvpW6aie71Z/EnGhcDBYwD2jJ4Qi3WncLLel3a5uM
yaFSCoCvjrN72/iHJYGng7RSitm36WwAZtPjukwbgIghVDDKmrs8E4dn/5InNgBe
Rp9xM2TbtPLWWz9fn0fWN/PicgH3tAuXxC5+QO3Nxa+Fi8yVzxYy15Psf2Ni4ZUR
Zs5o3xiil1YYHBqWKTG5fFAvIraXF2ZARjKkrGStKK5d8lT+zWXuw+0DgMaL1/UZ
MXPtuwKrAgMBAAECggEAaJbqNwyuAal+PkRxHXGzfle57ZUSxcqrm+4Eo8L0DtJG
zEkRmEr4XIkmSyHfufZYMer0Qyt8B50rRNULufculBBCPNKsFxoe7zw9lx3KOSds
jYYpE3AqA8em2zmFRZOWx3ynWBsUE5B+x7OiQ6F26y93g21tBu92u/z45IAhT4R4
T+kbEtFu3RqIFodjX9gf///Kvl4smlA53mjy+3PO154xnD/1WsDIK4MWNMzzh0MI
IMN6mtxe5bUqU4hn9bxFZiKWLQ8Q9Dntk1U1ThHOPIYACef+QhscipAxRK9U6esq
n2uEwvGuinM+LHcnhYy0YP5vtw20jok6GQbqzEzTkQKBgQDpmeHjFxjWwU00AOnI
MCbaKdAlOAbbIRer1Ian6fXlZjP74p19QeSEomiVDjKAoAGsMmdEK/kK1zO3T6k1
hl13EMtqYDRcIobf1cq1ClcLULYyE+DGqyP1TzIZxFXlk5eqf3HJounnUT2wIMUw
WoyTYKNTnAk+bxzXBFhgWUp6cwKBgQDRR/YlDu3VXRm8jOM3Y4QbNoolIGUpc4II
D5iZ1AkJoyhNIO9ENiZMAU+7jMf8nar5QW7kq5IDr4ZZzdVxFGuodYuqFPfCLhUK
e8YIVASECrukYm9g06qofk/ra/ykNr549HlJV99ANntjsOch6XwkdSiPm6aqLnpH
wPNTGxQw6QKBgDngGzv1I/1JBQSmWUV00Jtqkpw2BlTSHRhAXmBJsdd0+9ojKhu3
cJN/3WNYkiCWA/QSxMz6DAioirKW9PhC4vM14P/o9+//yeS5BjDWb/xoscs0a5Mt
IYqMZYBGyXVInOHsE1f+me7qjNsPM2uoc32sCqsTVKL4Sm/nLrIoTTCLAoGBAM0V
xvnT4m+nV5Q1QGjEBe6hCMmPMHNpdTCvD+0XI3AlSlYjAzYGFot+8YKqWESOwcCX
RbOjCmjANlmE4zh4OXQRFLes6oqInCf02UDKDM7UscNKjzkE1AVgGrNq1F6cIxXn
BYBBM0761PoBns7Vvsj/YqswbifxefUc+ZYkQCoZAoGBAIhM8IxxyjFJV06clX6F
kK5NFp2/ys8nMvt6nq8tdU+sm/jIN5EBJQMfdDnyjwEywrBZSsGLnY/DgIEREB80
2eeWE0+/G94HEIgGIMTRcGsgut/noYfcNNIAePV9YyVk/eqfU+1wmzto2GIIG49r
Wuho33TrTQFOv5oQO4J0LErw
-----END PRIVATE KEY-----
'''

resource secret 'core/Secret@v1' = {
  metadata: {
    name: 'super-secret'
  }
  stringData: {
    key: key
    cert: certificate
  }
}

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-gateway'
  location: location
  properties: {
    environment: environment
  }
}

resource gateway 'Applications.Core/gateways@2022-03-15-privatepreview' = {
  name: 'gtwy-gtwy'
  location: location
  properties: {
    application: app.id
    tls :{ 
      sslPassThrough:true 
    } 
    routes: [
      {
        destination: frontendRoute.id
      }
    ]
  }
}

resource frontendRoute 'Applications.Core/httpRoutes@2022-03-15-privatepreview' = {
  name: 'gtwy-front-rte'
  location: location
  properties: {
    application: app.id
    port: 81
  }
}

resource frontendContainer 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'gtwy-front-ctnr'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
      env: {
        TLS_KEY: base64ToString(secret.data.key)
        TLS_CERT: base64ToString(secret.data.cert)
      }
      ports: {
        web: {
          containerPort: port
          provides: frontendRoute.id
        }
      }
      readinessProbe: {
        kind: 'tcp'
        containerPort: port
      }
    }
  }
}
