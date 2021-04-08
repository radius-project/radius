FROM node:14
WORKDIR /usr/src/app
COPY package*.json servicebus-sender.js ./
RUN npm install
EXPOSE 3000
CMD ["node", "servicebus-sender.js"]
