package servicecontext

/*

Headers examples

{
  "Accept": [
    "application/json"
  ],
  "Accept-Encoding": [
    "gzip, deflate"
  ],
  "Accept-Language": [
    "en-US"
  ],
  "Content-Length": [
    "305"
  ],
  "Content-Type": [
    "application/json; charset=utf-8"
  ],
  "Referer": [
    "https://api-dogfood.resources.windows-int.net/subscriptions/a1301b53-5263-4e71-9bb4-5b406235a42c/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0?api-version=2022-03-15-privatepreview"
  ],
  "Traceparent": [
    "00-af9611048df2134ca37c9a689c3a6da3-f55f6b37134d0e40-01"
  ],
  "User-Agent": [
    "ARMClient/1.6.0.0"
  ],
  "Via": [
    "1.1 Azure"
  ],
  "X-Azure-Requestchain": [
    "hops=1"
  ],
  "X-Fd-Clienthttpversion": [
    "1.1"
  ],
  "X-Fd-Clientip": [
    "2001:4898:80e8:1:449b:f928:e40a:a351"
  ],
  "X-Fd-Edgeenvironment": [
    "Edge-Prod-CO1r5b"
  ],
  "X-Fd-Eventid": [
    "79C45A12DDEC4F8B80B65BB76819E49D"
  ],
  "X-Fd-Impressionguid": [
    "DC5AD6A4683244A39397525478EFB800"
  ],
  "X-Fd-Originalurl": [
    "https://api-dogfood.resources.windows-int.net:443/subscriptions/a1301b53-5263-4e71-9bb4-5b406235a42c/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0?api-version=2022-03-15-privatepreview"
  ],
  "X-Fd-Partner": [
    "AzureResourceManager_Test"
  ],
  "X-Fd-Ref": [
    "Ref A: 79C45A12DDEC4F8B80B65BB76819E49D Ref B: CO1EDGE1511 Ref C: 2022-03-22T18:54:50Z"
  ],
  "X-Fd-Revip": [
    "country=United States,iso=us,state=Washington,city=Redmond,zip=98052,tz=-8,asn=3598,lat=47.6786,long=-122.123,countrycf=8,citycf=8"
  ],
  "X-Fd-Routekey": [
    "463075349"
  ],
  "X-Fd-Socketip": [
    "2001:4898:80e8:1:449b:f928:e40a:a351"
  ],
  "X-Forwarded-For": [
    "10.240.0.7"
  ],
  "X-Forwarded-Host": [
    "westus3.rp.radius-dogfood.azure.com"
  ],
  "X-Forwarded-Port": [
    "443"
  ],
  "X-Forwarded-Proto": [
    "https"
  ],
  "X-Forwarded-Scheme": [
    "https"
  ],
  "X-Ms-Activity-Vector": [
    "IN.0P"
  ],
  "X-Ms-Arm-Network-Source": [
    "PublicNetwork"
  ],
  "X-Ms-Arm-Request-Tracking-Id": [
    "5f0b3a06-1890-4693-867e-110d454739ad"
  ],
  "X-Ms-Arm-Resource-System-Data": [
    "{\"lastModifiedBy\":\"billtest401628@hotmail.com\",\"lastModifiedByType\":\"User\",\"lastModifiedAt\":\"2022-03-22T18:54:52.6857175Z\"}"
  ],
  "X-Ms-Arm-Service-Request-Id": [
    "7c1ff604-803e-4bc2-9bb9-4bc7d1666424"
  ],
  "X-Ms-Client-Acr": [
    "1"
  ],
  "X-Ms-Client-Alt-Sec-Id": [
    "1:live.com:0006000017E4D539"
  ],
  "X-Ms-Client-App-Id": [
    "1950a258-227b-4e31-a9cf-717495945fc2"
  ],
  "X-Ms-Client-App-Id-Acr": [
    "0"
  ],
  "X-Ms-Client-Audience": [
    "https://management.core.windows.net/"
  ],
  "X-Ms-Client-Authentication-Methods": [
    "pwd"
  ],
  "X-Ms-Client-Authorization-Source": [
    "RoleBased"
  ],
  "X-Ms-Client-Family-Name-Encoded": [
    "UHJvamVjdA=="
  ],
  "X-Ms-Client-Given-Name-Encoded": [
    "UmFkaXVz"
  ],
  "X-Ms-Client-Identity-Provider": [
    "live.com"
  ],
  "X-Ms-Client-Ip-Address": [
    "147.243.137.236"
  ],
  "X-Ms-Client-Issuer": [
    "https://sts.windows-ppe.net/db817f8b-0402-4553-823f-bec90a442678/"
  ],
  "X-Ms-Client-Location": [
    "centralus"
  ],
  "X-Ms-Client-Object-Id": [
    "78ff3401-f833-4eff-aa15-d66e4f28b1a4"
  ],
  "X-Ms-Client-Principal-Group-Membership-Source": [
    "Token"
  ],
  "X-Ms-Client-Principal-Id": [
    "0006000017E4D539"
  ],
  "X-Ms-Client-Principal-Name": [
    "live.com#billtest401628@hotmail.com"
  ],
  "X-Ms-Client-Puid": [
    "0006000017E4D539"
  ],
  "X-Ms-Client-Request-Id": [
    "d7470fe1-a8b8-409a-9128-59d19bdc690d"
  ],
  "X-Ms-Client-Scope": [
    "user_impersonation"
  ],
  "X-Ms-Client-Tenant-Id": [
    "db817f8b-0402-4553-823f-bec90a442678"
  ],
  "X-Ms-Client-Wids": [
    "62e90394-69f5-4237-9190-012177145e10, b79fbf4d-3ef9-4689-8143-76b194e85509"
  ],
  "X-Ms-Correlation-Request-Id": [
    "d7470fe1-a8b8-409a-9128-59d19bdc690d"
  ],
  "X-Ms-Home-Tenant-Id": [
    "db817f8b-0402-4553-823f-bec90a442678"
  ],
  "X-Ms-Request-Id": [
    "d7470fe1-a8b8-409a-9128-59d19bdc690d"
  ],
  "X-Ms-Routing-Request-Id": [
    "CENTRALUS:20220322T185452Z:5f0b3a06-1890-4693-867e-110d454739ad"
  ],
  "X-Original-Forwarded-For": [
    "2001:4898:80e8:1:449b:f928:e40a:a351"
  ],
  "X-Real-Ip": [
    "10.240.0.7"
  ],
  "X-Request-Id": [
    "a37c9e84a94e98175f75730020df4a31"
  ],
  "X-Scheme": [
    "https"
  ]
}
*/
