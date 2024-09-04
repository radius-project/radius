extension radius

@description('Specifies the location for resources.')
param location string = 'local'

@description('Specifies the environment for resources.')
param environment string

@description('Specifies the port for the container resource.')
param port int = 3000

@description('Specifies the image for the container resource.')
param magpieimage string

resource app 'Applications.Core/applications@2023-10-01-preview' = {
	name: 'corerp-resources-gateway-dns'
	location: location
	properties: {
		environment: environment
	}
}

resource gateway 'Applications.Core/gateways@2023-10-01-preview' = {
	name: 'http-gtwy-gtwy-dns'
	location: location
	properties: {
		application: app.id
		routes: [
			{
				path: '/'
				destination: 'http://frontendcontainerdns:3000'
			}
			{
				path: '/backend1'
				destination: 'http://backendcontainerdns:3000'
			}
			{
				// Route /backend2 requests to the backend, and
				// transform the request to /
				path: '/backend2'
				destination: 'http://backendcontainerdns:3000'
				replacePrefix: '/'
			}
		]
	}
}

resource frontendcontainerdns 'Applications.Core/containers@2023-10-01-preview' = {
	name: 'frontendcontainerdns'
	location: location
	properties: {
		application: app.id
		container: {
			image: magpieimage
			ports: {
				web: {
					containerPort: port
				}
			}
			readinessProbe: {
				kind: 'httpGet'
				containerPort: port
				path: '/healthz'
			}
		}
		connections: {
			backendcontainerdns: {
				source: 'http://backendcontainerdns:3000'
			}
		}
	}
}

resource backendcontainerdns 'Applications.Core/containers@2023-10-01-preview' = {
	name: 'backendcontainerdns'
	location: location
	properties: {
		application: app.id
		container: {
			image: magpieimage
			env: {
				gatewayUrl: {
					value: gateway.properties.url
				}
			}
			ports: {
				web: {
					containerPort: port
				}
			}
			readinessProbe: {
				kind: 'httpGet'
				containerPort: port
				path: '/healthz'
			}
		}
	}
}

