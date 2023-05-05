import radius as radius

@description('Specifies the environment for resources.')
param environment string

@description('Specifies the port for the container resource.')
param port int = 3000

@description('Specifies the image for the container resource.')
param magpieimage string

param certificate string = '''
-----BEGIN CERTIFICATE-----
MIIFljCCA34CCQCClM1GARh19zANBgkqhkiG9w0BAQsFADCBjDELMAkGA1UEBhMC
VVMxCzAJBgNVBAgMAldBMRAwDgYDVQQHDAdSZWRtb25kMRIwEAYDVQQKDAlNaWNy
b3NvZnQxDjAMBgNVBAsMBUF6dXJlMRIwEAYDVQQDDAlsb2NhbGhvc3QxJjAkBgkq
hkiG9w0BCQEWF3dpbGxzbWl0aEBtaWNyb3NvZnQuY29tMB4XDTIzMDQyNDA0MDcy
M1oXDTI0MDQyMzA0MDcyM1owgYwxCzAJBgNVBAYTAlVTMQswCQYDVQQIDAJXQTEQ
MA4GA1UEBwwHUmVkbW9uZDESMBAGA1UECgwJTWljcm9zb2Z0MQ4wDAYDVQQLDAVB
enVyZTESMBAGA1UEAwwJbG9jYWxob3N0MSYwJAYJKoZIhvcNAQkBFhd3aWxsc21p
dGhAbWljcm9zb2Z0LmNvbTCCAiIwDQYJKoZIhvcNAQEBBQADggIPADCCAgoCggIB
AK/QDR+Cqaoi/q5Yv8vibcGyxVAEUSX6Szv1Vzr5ai0tDIUVjt9OjDRooyF/i2Zo
kBlu/zqaC0/ICmjLXrpwgmAA/lyTLH5wM8rEEOygGWA0pP3g69/7dBBHcnlIX621
taJUzcZNugiQgWsvhALKmrPbPwklROUz3axT87jipCXwFwhPAZbPvG/O/T8dg27i
L2g+x2bZnXYBNsOWUgbfyL/GC8AxxZK4WTL+FBBtKWBIAcrrsj0Adr1ByQ917sMG
rIpgcH3pGjkw3mjQBURvMEfn8b8GTi14JogXCHNclJT9IqT4P8gTGsR5SeAuJ1LZ
ILwRPl6FCIw4m/BH9ym61npdJYgyzG6CCZ1L6xjh0d7lI/IiEMP1REFPtx5pJxtF
tLQ1W8Vv/7Z4fka++I3HAzju/My+DwJ+R/lE/+jth6V5Zgc604CDFoph35g5e0Qs
zKFWr4Xlvx5V6p9mkaA1KrfOfOhvn6YQVaTqYFFg7BbcTWRFPoNpwUrgX4ArxSUK
gd96jLqocfYE43wEcds31RSDwtrMJoANn+dyHX4YbW/fKBI6bsCZHen45JAUWiUy
BSZgbTO9mvcBWpNnD1kmhpE0s1ufTn2RY2xMN2j5oEeif5RT7Pl3C74UPnN0CXRb
k8uOrw7kZ4NHDh9O70UDdgCw2m/xVA4U1etdZ5/FTqanAgMBAAEwDQYJKoZIhvcN
AQELBQADggIBAKtjf3l8yIFPocb60VOoQjTsEp+eU77hodFro1F8LQ7ZNDaOi4cN
u1D8pbyUi/zXMqLsLtRsZ9Vj43VM56QXeod2QyxMMnW9+fmlCXzOJNw1wHE10Era
FC54Nfq6YI3CYE/f4EL+JHq1ch9+7TRKtAuavrmTmzmVodpU+/mhNiBPEe/OAfEO
bwhauoqO7B8UJiggDufkgim4FoAjQWuKDPWA3uF5CY+yGKvoEAG0Fn67tygBfEbD
3FIjnoYvl1LjvN9ZvIByl6RnVXX8IinhsU87Aj5ZTkX1sZ5XYGR7Gw1XPWRgNdaE
Vbk0x2t2mp1DVp/y/ixLLrC0LmuDor8JEST72CZxLrMIcuJeg3wWS+btN4IRKzr2
RQlX2+CQd7gSFYNA0/ZGuGCo3Bme1p2DhEkzuwweSC9Fw1L09RZtbsc0TLHC4mvf
SOcYWDuSrLdhj21nlEuvyVxFsGnUD5esQ+2Kfma6Ceg7vQC2jKqomZ28Nwmnetx7
fyQ1Y5eGleXKA+s02/4zEXGV1ygWxk3CSKxa+4oEcJtE1gxrSqsSe+FgIDft8IC1
PEVTd8pOTr2gHhBVFIkGo5C68JKe6nV/OKTNWn0xLVZvUIrdAhppcH/Jd7nmdSt8
pZkXJkkM7IR9cHCXHkGDl1grgfDSqlas4Y4ClJkiKjsKwP75RodNsD4H
-----END CERTIFICATE-----
'''

param key string = '''
-----BEGIN PRIVATE KEY-----
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
+Cp2E4G9rjnFoab/Fs9mEVUGUl6N0mA=
-----END PRIVATE KEY-----
'''

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-gateway-tlstermination'
  properties: {
    environment: environment
  }
}

// Create new appcert kubernetes secret.
resource appCert 'Applications.Core/secretStores@2022-03-15-privatepreview' = {
  name: 'appcert'
  properties: {
    application: app.id
    type: 'certificate'
    resource: 'appcert'
    data: {
      'tls.key': {
        value: key
      }
      'tls.crt': {
        value: certificate
      }
    }
  }
}

resource gateway 'Applications.Core/gateways@2022-03-15-privatepreview' = {
  name: 'tls-gtwy-gtwy'
  properties: {
    application: app.id
    tls: { 
      minimumProtocolVersion: '1.2'
      certificateFrom: appCert.id
    } 
    routes: [
      {
        path: '/'
        destination: frontendRoute.id
      }
    ]
  }
}

resource frontendRoute 'Applications.Core/httpRoutes@2022-03-15-privatepreview' = {
  name: 'tls-gtwy-front-rte'
  properties: {
    application: app.id
    port: 443
  }
}

resource frontendContainer 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'tls-gtwy-front-ctnr'
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
      readinessProbe: {
        kind: 'tcp'
        containerPort: port
      }
    }
  }
}
