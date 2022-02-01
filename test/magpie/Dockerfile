# Issues on M1 mac: https://github.com/docker/for-mac/issues/5831
FROM node:16

WORKDIR /usr/src/app

COPY package*.json ./
RUN npm install

COPY . .
RUN npm run build

EXPOSE 3000
CMD [ "node", "dist/server.js" ]