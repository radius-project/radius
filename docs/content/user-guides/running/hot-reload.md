---
type: docs
title: "Hot reload in Radius local environments"
linkTitle: "Hot reload"
description: "Guides on how to quickly test out applications locally with hot reload"
weight: 200
---

Hot reload lets you automatically refresh your running application based on a file being updated.

## Example: NodeJS + Nodemon

Nodemon makes it easy to refresh your application when you make changes to your source code.

### Dockerfile configuration

```dockerfile
FROM node:14-alpine

USER node
RUN mkdir -p /home/node/app
WORKDIR /home/node/app

COPY --chown=node:node package*.json ./
RUN npm ci
COPY --chown=node:node . .

EXPOSE 3000
ARG ENV=development
ENV NODE_ENV $ENV
CMD ["npm", "run", "watch"]
```

In addition, don't forget that for hot reloading you need to change your package.json file to support `watch` such as this:

```json
{
  "name": "node-service",
  "version": "0.0.0",
  "private": true,
  "scripts": {
    "start": "node ./bin/www",
    "watch": "nodemon ./bin/www"
  },
  "dependencies": {
    "axios": "^0.22.0",
    "cookie-parser": "~1.4.4",
    "debug": "~2.6.9",
    "express": "~4.16.1",
    "http-errors": "~1.6.3",
    "jade": "~1.11.0",
    "morgan": "~1.9.1"
  },
  "devDependencies": {
    "nodemon": "^2.0.15"
  }
}

```

## Configure path mounts

//TODO