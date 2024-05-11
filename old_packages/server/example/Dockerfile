FROM node:20-alpine3.16

WORKDIR /app

COPY package.json package.json
COPY index.mjs index.mjs

RUN yarn

CMD [ "node", "index.mjs" ]