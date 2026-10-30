extension radius

@description('Radius Environment ID, supplied by the selected workspace.')
param environment string

@description('Portable workload image, unchanged between deployment targets.')
param image string = 'nginx:1.27-alpine'

@description('Portable TCP destination port, used for the route-only update.')
param routePort int = 80

var peerScript = '''
import os,socket,time,json
host=os.environ["TARGET_HOST"]
while True:
  for port in (80,8080):
    try:
      with socket.create_connection((host,port),timeout=2) as connection:
        connection.sendall(b"GET / HTTP/1.1\r\nHost: demo\r\nConnection: close\r\n\r\n")
        chunks=[]
        while True:
          chunk=connection.recv(4096)
          if not chunk: break
          chunks.append(chunk)
        response=b"".join(chunks).decode(errors="replace")
      print(json.dumps({"host":host,"port":port,"ok":"delegated provisioning demo" in response}),flush=True)
    except OSError:
      print(json.dumps({"host":host,"port":port,"ok":False}),flush=True)
  time.sleep(3)
'''

resource application 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'portable-demo'
  properties: {
    environment: environment
  }
}

resource server 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'server'
  properties: {
    environment: environment
    application: application.id
    containers: {
      web: {
        image: image
        command: ['/bin/sh', '-ec']
        args: [
          'printf \'server { listen 80; listen 8080; listen 9090; location / { return 200 "delegated provisioning demo\\\\n"; } }\\n\' > /etc/nginx/conf.d/default.conf; exec nginx -g \'daemon off;\''
        ]
        ports: {
          http: {
            containerPort: 80
          }
          alternate: {
            containerPort: 8080
          }
          metrics: {
            containerPort: 9090
          }
        }
      }
    }
  }
}

resource ingress 'Radius.Compute/routes@2025-08-01-preview' = {
  name: 'ingress'
  properties: {
    environment: environment
    application: application.id
    kind: 'TCP'
    rules: [{
      matches: []
      destinationContainer: {
        resourceId: server.id
        containerName: 'web'
        containerPort: routePort
      }
    }]
  }
}

resource peer 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'peer'
  properties: {
    environment: environment
    application: application.id
    containers: {
      probe: {
        image: 'python:3.12-alpine'
        command: ['python', '-u', '-c']
        args: [peerScript]
        env: {
          TARGET_HOST: {
            value: ingress.properties.listener.hostname
          }
        }
      }
    }
  }
}

output serverId string = server.id
output peerId string = peer.id
output listener object = ingress.properties.listener
