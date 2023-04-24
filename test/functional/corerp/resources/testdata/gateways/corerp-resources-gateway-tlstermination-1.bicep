import kubernetes as kubernetes {
  kubeConfig: ''
  namespace: 'default'
}

@description('Specifies the certificate for the container resource.')
param certificate string = '''
-----BEGIN CERTIFICATE-----
MIIDxDCCAqwCCQCTa/2H74gH9zANBgkqhkiG9w0BAQsFADCBozELMAkGA1UEBhMC
VVMxEzARBgNVBAgMCkNhbGlmb3JuaWExEDAOBgNVBAcMB0ZyZW1vbnQxEjAQBgNV
BAoMCU1pY3Jvc29mdDEOMAwGA1UECwwFQUFDVE8xIjAgBgNVBAMMGWNvcmVycC1y
ZXNvdXJjZXMtZ2F0ZXdheXMxJTAjBgkqhkiG9w0BCQEWFm5pdGh5YXN1QG1pY3Jv
c29mdC5jb20wHhcNMjIxMjAxMTkzNDE1WhcNMjMxMjAxMTkzNDE1WjCBozELMAkG
A1UEBhMCVVMxEzARBgNVBAgMCkNhbGlmb3JuaWExEDAOBgNVBAcMB0ZyZW1vbnQx
EjAQBgNVBAoMCU1pY3Jvc29mdDEOMAwGA1UECwwFQUFDVE8xIjAgBgNVBAMMGWNv
cmVycC1yZXNvdXJjZXMtZ2F0ZXdheXMxJTAjBgkqhkiG9w0BCQEWFm5pdGh5YXN1
QG1pY3Jvc29mdC5jb20wggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQDX
ZbUuwAYcqnToURg7Q5xxCpc1VtLK6EapWvsOmEVjGzZBqs76DnkLRxCbGWZXq9RJ
CZ/v6vC/xILmUPe/ZfqYs1LpU5XzQ7wMsSIKOL8r9rgsfq28v7QZC+lOtzJksh8F
HwyacPwE9NFsivhI+m1D0nh9DA5JZhvn56TNz3D7AgZrId0Jw68bRbmr+q7IWTvN
ylpCxmseoDBxS2SC5jLrCB/cXpuQ4grJzStxUra4P/iJMm3Gwl7O+c36ZL7TH2Nq
P4dT9w6qvOSGavGCYAftUtxREsvnHmnVdLaZtYWo+sNh2V17dGe8pgx7GJ8y60H2
KiHEczGvrVmzPA+sS8ulAgMBAAEwDQYJKoZIhvcNAQELBQADggEBAADJXfNd+gf8
/Vt2WleqhBXTqNwoOhkx5YFGwOHnRI6Y0CTvO67+iAaOld7kPYqmq2ahgFGaWzEt
LSe9i3dnm/UOEtB3RNmrtTmPkjCwTyIwIvEtpQwoXXVZK+Bm3Zb3wlkToJo0CdGs
AUS5rTVh9I1R7yCDa5lrfftUGmjWkxBP3HIiIOqx6mOF79a87LM8PBHYNi2wOdvh
ZEgUYLioogwan0o+f3DbbwKfNTDH+whvBryLgFGfcOiCAD+4Q6sLiV9bv64jnimg
qubIbGV+PWSZ6C6+cZ41IKbBbleQItI1LTtvGCXhW4jNXGGpEljKvk4jRucQt3C4
kPC4dDjt2zU=
-----END CERTIFICATE-----
'''

param key string = '''
-----BEGIN PRIVATE KEY-----
MIIEvwIBADANBgkqhkiG9w0BAQEFAASCBKkwggSlAgEAAoIBAQDXZbUuwAYcqnTo
URg7Q5xxCpc1VtLK6EapWvsOmEVjGzZBqs76DnkLRxCbGWZXq9RJCZ/v6vC/xILm
UPe/ZfqYs1LpU5XzQ7wMsSIKOL8r9rgsfq28v7QZC+lOtzJksh8FHwyacPwE9NFs
ivhI+m1D0nh9DA5JZhvn56TNz3D7AgZrId0Jw68bRbmr+q7IWTvNylpCxmseoDBx
S2SC5jLrCB/cXpuQ4grJzStxUra4P/iJMm3Gwl7O+c36ZL7TH2NqP4dT9w6qvOSG
avGCYAftUtxREsvnHmnVdLaZtYWo+sNh2V17dGe8pgx7GJ8y60H2KiHEczGvrVmz
PA+sS8ulAgMBAAECggEBALieKElFtPdk8occ2tQRA0mwdiH1pP7dT5Ngs8aypOZp
MHvgVz7koMMVyhnmD14dnPptEXSlvmvflwKpa2/VjJDNQsdSKTg0Wj3WpQJ12QVp
ljos6eTZuxesqfAZ/UtpkETnc/n71Ua6P7X09xI18ukqwLMNXkFzD4AZf5wXrRK1
+nQkvQiLWYAytu7wjfyKKZIATSNYQ3KoztnO35vLfXqxS7+ijOvoz8oDj1sZ2g09
PqAckCCKwcdTXCnzAb61jvluvWNqrWa1EEHHRPfM/lu/iAgtK1+OACFolc+GlzSc
zr1VOglPrPU3v/Lemn84Yr0TWdHKjzq4avg7q/+8xoECgYEA+HBvYugss+wgXGu7
o8DtO2ropLpUzAVPPdmFJ27D7k1Et2Mwj2bqJA/xGgXUnC6ysPD+Rk1WezOJ/meO
YOQihJklmAI8pxOSxfhJlgwix7+kA/p2FfsWvNn7aNEvB+m1lsrKW6o2tXkLtELY
VZyJ0zb6mHaNyuvGvXLpvPGpk30CgYEA3fPYsw6JuAcmIkUwWIAVb+o6lTtFYilu
G7m100wxwercxdLSu3fNJ2kRh5KlACm9WiDEKvzTjQr/Wp+6xL3J9x8ii23D4WCA
Jm+LARVI3xVkc1jUqOJqqh95arH0g2uT1qCOdDcsftDTSfhqz6iiNk0KNPfnnuzY
rNZbvzLWQUkCgYBA5xaiTydGhbxaiKaHfCI9sItAZZE7j3OJI+deStiSy8rU4evQ
usEWVLfW5YkKmESEZyD2esPKAcfeF22hsFe4Lk4c7RCtUTa500heE6OObWlKxMbO
rT7ebU/5rRRNS+ftkeLVmZ0bQZkmKYRcsT1sWWOUKvyV84yC959KhhOX/QKBgQDG
uSuOtjeMc6orCPPOaW/IMlmdf+IRj7KsVEx+ETyDuXtODALuIsemv6YYUq41RSnq
Zmf9bT0kjXIwe89Hk/4eqtvNJsw5IKPcxgYZRCtowcicli5hv8ds5p1ZcFfSyyEl
C8BAQZ4vNV3YXvmTUBNctwGqh0P0wW8G4S5oNGYtMQKBgQC6VEGQLERG/XPcyh6v
W77HNMuPHy8yopAZ4bCfqP2gOdEKsj34N+OTV5K8dTEITp3hYsTx0TmrC55owpsZ
3UtJRUdN3QE4mT4qlSzxZWJ+6F9J85UUbd1/gRMA+GfB4blw+I5vX2P30YIca5g5
QYP6qDTvyLieC2CKFFQbfll3jQ==
-----END PRIVATE KEY-----
'''

resource ns 'core/Namespace@v1' = {
  metadata: {
    name: 'default-corerp-resources-gateway-tlstermination'
  }
}

resource secret 'core/Secret@v1' = {
  metadata: {
    name: 'tlstermination-secret'
    namespace: 'default-corerp-resources-gateway-tlstermination'
  }
  data: {
    tls: {
      key: key
      cert: certificate
    }
  }
  type: 'kubernetes.io/tls'
}
