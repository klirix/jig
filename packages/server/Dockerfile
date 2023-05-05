FROM node:alpine

WORKDIR /var/app

COPY package.json package.json

RUN yarn install

COPY . .

EXPOSE 8888
CMD [ "yarn", "start" ]
