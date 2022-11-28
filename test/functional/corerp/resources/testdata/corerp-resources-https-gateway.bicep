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
MIIDmDCCAoACCQDU02uSnUss8zANBgkqhkiG9w0BAQsFADCBjTELMAkGA1UEBhMC
VVMxEzARBgNVBAgMCmNhbGlmb3JuaWExEDAOBgNVBAcMB2ZyZW1vbnQxDTALBgNV
BAoMBHRlc3QxDTALBgNVBAsMBHRlc3QxFDASBgNVBAMMC3Byb2plY3QuY29tMSMw
IQYJKoZIhvcNAQkBFhRuaXRoeWF0c0BvdXRsb29rLmNvbTAeFw0yMjExMDEyMTE1
MjhaFw0yMzExMDEyMTE1MjhaMIGNMQswCQYDVQQGEwJVUzETMBEGA1UECAwKY2Fs
aWZvcm5pYTEQMA4GA1UEBwwHZnJlbW9udDENMAsGA1UECgwEdGVzdDENMAsGA1UE
CwwEdGVzdDEUMBIGA1UEAwwLcHJvamVjdC5jb20xIzAhBgkqhkiG9w0BCQEWFG5p
dGh5YXRzQG91dGxvb2suY29tMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKC
AQEA0MDt02vYOXPOkyKPDyaXMZyy7etzROwcXvCOD16bfI4qXjhyghwQ/UXsfjlB
/FIxahF3YfiB4R6tsDA9ZdPDQy4NS/+mm8Gy1/NmQb/D3gpfjQkEoMG9tscroQYM
ytaA9LBcJqP2SzhRAM3zIzK+zqllFHB0Unmgfi2+qDkXDUPcQNtasWCfCFAKuaGY
TzCplPs6/LBsw6W+y4vJ5q02W6IJCpwBqazfF5LPGAY8mMuwgw1t+e9bGjZZ6QfX
wztj64R4UZM3HmLIdjua3DZHueQ8tJRC6A09oGipPysC8mvnqvehFQsQbiNHbuE0
ucJpm/JLDffsZb2GJxRN9ZmCiwIDAQABMA0GCSqGSIb3DQEBCwUAA4IBAQBOQAeV
i2YzzbVuHQOL93OrP94sGq3sJbvVgBd0UjtqmoEsFqnvWrRZDu1qVUtymwHQmaU3
F347hE1RLQJn7n2KP3kEBMhOLwa/T1if3gADc3/1JAKye6U4VHZplHdIOCgyrNUo
FAGYOOk+LQ1UK6mr4htrd25QSsoZnxF3fHrqO7qTXKczFLyi+v1y3zNhMzRVGvQ7
CYgINakobn0C+YwoIf9SKABMTYvQwaqghgglUvzlJTqYzeFQCwdsmCMuToicC4fi
i3eQIafF7BZvVBf0F8mrtvMAhKnVsYqQMN0GOIZ9YKfF0kFf5DcltoS0ulcFFxJb
ZnFWGq+Vs/5XXtDa
'''

param key string = '''
MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQDQwO3Ta9g5c86T
Io8PJpcxnLLt63NE7Bxe8I4PXpt8jipeOHKCHBD9Rex+OUH8UjFqEXdh+IHhHq2w
MD1l08NDLg1L/6abwbLX82ZBv8PeCl+NCQSgwb22xyuhBgzK1oD0sFwmo/ZLOFEA
zfMjMr7OqWUUcHRSeaB+Lb6oORcNQ9xA21qxYJ8IUAq5oZhPMKmU+zr8sGzDpb7L
i8nmrTZbogkKnAGprN8Xks8YBjyYy7CDDW3571saNlnpB9fDO2PrhHhRkzceYsh2
O5rcNke55Dy0lELoDT2gaKk/KwLya+eq96EVCxBuI0du4TS5wmmb8ksN9+xlvYYn
FE31mYKLAgMBAAECggEAK/0boHuPOrwOga68mqK1JX0xrzT4O0PNzqu+I7r55MtI
XkZiyswDQHullAuYvgTL6N/5Wim1pKyESSZBKd3vvY5MuwEKKLQubZcaqywvp/Bj
piKKWR26TnO1296cf3mn/ufS40mVstARMaw0WextjLrhU+dGe8KpcS1OicBN/Ts4
FsRnYyp8j6ziR0KEAkIJnWrgDdGNKe7H4gAYdxKFVBcun6AB7A+LGG9zgLFJ9Px7
d8/LcIz9EAfg5t9k59Qj+FW+kEPiEUl1xpBrhiwj18hwfRUAslnBgQo2DF0+1zFA
dlEhUBmgNKoUV7YNAf4vhfXUKBbnjsv2y6kcgLk1wQKBgQDoORpJj2TZpSG0e3SJ
dkrQQnomWCeeeDzbztnZZGQV6/T1GM1103t/NXD4XJdRdMAMvb/48sQAQQvTbHK5
fxH6tKVA9p+VuWjF8aj2HthgUYeWHUmp8MbMh7XlmZ/Tor2MMOIGZQk9AHnlGDJc
BsYXW27hCcRGibtGJ3qYu35bsQKBgQDmIKi+PfCnWqCHB8XbHSa7Vsu4uNJOeJFN
J3rDjYm9gomfEgzNiBYtbNrqkHx5zHeXrc+aUZHyieIZ9JZiBBPWWmUJ8FEgWmtK
QblGCk2ERYssn910PKue+uVG18r3w/H83txUtHnLJ8VR2CPgq1z71BUKegrMWS5z
UQ4U/mJc+wKBgBbfYPZz2DQTrrEvI7hSXWYL1iomrqhOIXho9E4UNENwfS0S51G+
pcBOzDS6MfFE9ZGLsvfbOXDo9zg4y0f3+xZdapVudSNzIp20grbTLO63uQoREmtZ
mssUZtcZfYOD2PWQ7wJAO1u1y0vESVmFFUfBqrchliJ4eGidhNa8SOLRAoGAXcE4
fikl/kiB1gForlg2C2TVIrDJnYapS9Glxj3HvBmOj+v+o02qG1+Z4K50x/pxTq5V
Qf2xhCqAnypyigQ3QMEbIO1zX8b2pw4XuV1BL35VsRyAUHbXRLHa7v3DhyWhVPBG
u4u7gvT1At8X3tRx0XcaC2alN5OtxPVk01DAKjkCgYEAwMUSJTrey8YZ8jZAB0EN
kNSEHWb4E5neoLA4UQElkTK17hzMKgQjvo08hiPn7D8562RVLWPV+5Tht6TEA2rC
G0mSuU4Ii6/jwHBZO6RgGrBOOXyxgDGC9PCE4NBN1FxORvnR1H3FOa7nxJJvydmi
uV4o57PzHryEzOIPAPcdPmM=
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
      ports: {
        web: {
          containerPort: port
          provides: frontendRoute.id
        }
      }
      env: {
        KEY: base64ToString(secret.data.key)
        CERT: base64ToString(secret.data.cert)
      }
      readinessProbe: {
        kind: 'httpGet'
        containerPort: port
        path: '/healthz'
      }
    }
  }
}
