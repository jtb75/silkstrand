# Build stage
FROM node:20-alpine AS builder

ARG VITE_CLERK_PUBLISHABLE_KEY=""
ENV VITE_CLERK_PUBLISHABLE_KEY=$VITE_CLERK_PUBLISHABLE_KEY

WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci

COPY . .
RUN npm run build

# Run stage
FROM nginx:stable-alpine

COPY --from=builder /app/dist /usr/share/nginx/html
# Template substituted at runtime by the nginx image's envsubst entrypoint
# (/docker-entrypoint.d/20-envsubst-on-templates.sh).
COPY nginx.conf.template /etc/nginx/templates/default.conf.template

# Defaults for local docker-compose; overridden in Cloud Run.
ENV PORT=80
ENV SILKSTRAND_API_URL=http://api:8080

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
