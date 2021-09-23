FROM node:14
WORKDIR /usr/src/app
COPY package*.json servicebus-receiver.js ./
RUN npm install
EXPOSE 3000
CMD ["node", "servicebus-receiver.js"]
