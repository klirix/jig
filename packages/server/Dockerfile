FROM node:alpine as builder

WORKDIR /var/app

COPY package.json package.json

RUN yarn install

COPY . .

FROM node:alpine

WORKDIR /var/app

COPY --from=builder /var/app/node_modules /var/app/node_modules

COPY . .

EXPOSE 8888

CMD ["yarn", "start"]
