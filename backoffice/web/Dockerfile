# Build stage
FROM node:20-alpine AS builder

WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci

COPY . .
RUN npm run build

# Run stage
FROM nginx:stable-alpine

COPY --from=builder /app/dist /usr/share/nginx/html
# Use template so nginx.conf can be generated from env vars at runtime.
# nginx's official image runs /docker-entrypoint.d/20-envsubst-on-templates.sh
# which substitutes ${VAR} in any *.template file under /etc/nginx/templates/.
COPY nginx.conf.template /etc/nginx/templates/default.conf.template

# Default values for local docker-compose (overridden in Cloud Run)
ENV PORT=80
ENV BACKOFFICE_API_URL=http://backoffice-api:8081

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
