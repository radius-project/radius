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
MIIDxDCCAqwCCQDHeIW6FEREizANBgkqhkiG9w0BAQsFADCBozELMAkGA1UEBhMC
VVMxEzARBgNVBAgMCkNhbGlmb3JuaWExETAPBgNVBAcMCFNhbiBKb3NlMRIwEAYD
VQQKDAlNaWNyb3NvZnQxDjAMBgNVBAsMBUFBQ1RPMSEwHwYDVQQDDBhjb3JlcnAt
cmVzb3VyY2VzLWdhdGV3YXkxJTAjBgkqhkiG9w0BCQEWFm5pdGh5YXN1QG1pY3Jv
c29mdC5jb20wHhcNMjIxMjAxMDMwODM1WhcNMjMxMjAxMDMwODM1WjCBozELMAkG
A1UEBhMCVVMxEzARBgNVBAgMCkNhbGlmb3JuaWExETAPBgNVBAcMCFNhbiBKb3Nl
MRIwEAYDVQQKDAlNaWNyb3NvZnQxDjAMBgNVBAsMBUFBQ1RPMSEwHwYDVQQDDBhj
b3JlcnAtcmVzb3VyY2VzLWdhdGV3YXkxJTAjBgkqhkiG9w0BCQEWFm5pdGh5YXN1
QG1pY3Jvc29mdC5jb20wggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQC0
wAKgHuqBkJkCKD2dXG2NO/Im0RqCmzKLRd0P2klFm2s85CzErW7WJTnrxswJip9m
gz2Slhn+xWLOVfE6YwMIeZWUyaiM4IdCZf69sT63Ri77O8b/aG1JHuvHgF6m4Zj1
l/SCAgXzepBHs3P+8+oNHNAZmqzcxlnpgny5mygRhKg6A96uV+J1esjZyXndHh0d
U6lM267P8IiV6p83KY//OKgxUsvMtC1Pz3fUKmLXyaL8iXayaFeIZhdpNdS8AwTW
GIjSZNA2o3h924DK4Kf6d+X+FT058iNZDO4uR8V8WwxcFW/VMDET4jbX2OAlzqd3
lUOIx75rzHWpV4KCMu4LAgMBAAEwDQYJKoZIhvcNAQELBQADggEBAKcqmxFffEzP
6X1mU+HU+t8ds3dxAtzryoUizpn9FuZygrVUeAR5Ar8jmCSr/CqjF527Yb7a00Se
CE4gsQVva/GztFASZHmzy3wkcm6JBpbZ5WL+24EreU2sZ/Wlz8TlMtuQSbP/vsy6
KEy9y1t5NMCkEK+rVj2MhBIaaO+wQ7hls+2EKh5dJiGlvanIMN5Uq5YeHxVKiopu
Bs22gwdNp8qA+5pg9rCTVmiNRVlbtqG4Tg0JtJ5rrl8tIwukab44bJE8itKlvBAJ
H4JKl482hHA6E+XSeMGGNrGiZBAVpPaspbrM44TmnMgcFW43o+5VY7LrCxK+9Yye
v3b8+vX6544=
-----END CERTIFICATE-----
'''

param key string = '''
-----BEGIN PRIVATE KEY-----
MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC0wAKgHuqBkJkC
KD2dXG2NO/Im0RqCmzKLRd0P2klFm2s85CzErW7WJTnrxswJip9mgz2Slhn+xWLO
VfE6YwMIeZWUyaiM4IdCZf69sT63Ri77O8b/aG1JHuvHgF6m4Zj1l/SCAgXzepBH
s3P+8+oNHNAZmqzcxlnpgny5mygRhKg6A96uV+J1esjZyXndHh0dU6lM267P8IiV
6p83KY//OKgxUsvMtC1Pz3fUKmLXyaL8iXayaFeIZhdpNdS8AwTWGIjSZNA2o3h9
24DK4Kf6d+X+FT058iNZDO4uR8V8WwxcFW/VMDET4jbX2OAlzqd3lUOIx75rzHWp
V4KCMu4LAgMBAAECggEAJhiWNRNvD9HfPgInQmR5vpvU5POLp914ILyf5Dh42w/v
4UyiVu3K/52nAJfM53HtONkOgDfc0MLfmWepyUmXGREvQsXiHZcxSwBeWbLi6hQD
0PX/clObPR8kSM84o+nGqHTXlxNAF9pMUKB1IVZdjVf2UH82Ue4Ig1v6V9Bo+I/n
gIyMQJoMcIrFbyaVjLVPqDOPpfIbMxC4fJYkz4bIHxjdT3GKZ24aEvxwRyEcNmGn
hs0zTpfcdjdBtqjV+GZuZGkbzdqB644jpxaSBN9r2hv0i4Tf+VJpzdbgRVMQ4YZY
b+fOF5WnUSwymVFZXfTB1kYnpCstZwEoz3RWccCNqQKBgQDd6FSAirbeRZQazqNa
FFiaun9ERIaRWDzDab5QGxA5CFw6qb/jcuxlnSe70sS82c6fQy/WvqBnSvRuChVU
UEwc1pcEvf/SenZSUcyxmMiqwFpnMSzfE1bLdgHSkmvpvL53cw2vyVSryfnZdAEN
0hesrI3r6k6qaWeuvJeCImk2TQKBgQDQhPJ2Oso7y2QrRjbPRTTZtI6s4GqXrDob
jOexBvEJT+nk3A0eaawsuJhKkkOwas9xzZytGJ89BZvFP0bRF3ktABb21p4Kbd9J
eUgmOD0UT0PnFAySmKdCA1DT9Ju/fiqfD9WeJEn/0LpG3M+GR4R/toyw0Esyn3ih
LgC9cuoRtwKBgH5XZa0dzQn18WHl3mlOBjhqEEWNAlTEOSxFCz7OeckO8nvP49ma
t+8Or+2nDa48EADrHtSUCf1lVo9EHGq5oOwWXTss9fcfFDjAK9u9khptk8sG23ZS
q2sBz/3Usa4NcR/PGK7J4PRB9YeSHXuB70q3n8H+0DUD+C0rYNONxftNAoGAJH2v
pMsjCxXMANq3yswUtKipc02Oud5VCO8+uLc7RWLrzrZHwXPCwszHMf2oxN3cUdEm
wxAVBevOV9V8Ail2dk6WtjnWzIJv2f7UhoO/BKfefTj//kOiuaW05nLfMsLUmKN/
wb4eCRuxDaek1Z38bRE4S9UX49MOnD5duMm8dr8CgYEAi7Uy9XLcyO0j5OQDrGhX
E4Wn5UbajW8HDBK18I3smiQhvzZ9Yq9PqsltntN9CUTIZZlWWck2uSZETbVOkAWW
aDaw9uG7QLUWLzp8sktVGv6EQyHaCaxfExyfI0lnYIXA1psCVEF32Q6+WAdlnP/v
X5wJMxma1V8UIgNyTwMimac=
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
  name: 'corerp-resources-gateways'
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
    port: 443
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
