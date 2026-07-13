FROM node:20-alpine AS builder
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
# Vite bakes VITE_-prefixed vars into the bundle at build time (not
# runtime), so the API base URL must be supplied as a build arg. Defaults to
# the host-exposed api-gateway port, matching how the app runs without
# Docker (see frontend/web/src/services/apiClient.js).
ARG VITE_API_BASE_URL=http://localhost:8079
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL
RUN npm run build

FROM nginx:1.27-alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
