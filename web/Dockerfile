# syntax=docker/dockerfile:1

# Builds the web app, then serves it from Nginx, which is also the way in to the API.
FROM node:24-alpine AS build
WORKDIR /src
RUN corepack enable
COPY package.json pnpm-lock.yaml ./
RUN --mount=type=cache,target=/root/.local/share/pnpm/store pnpm install --frozen-lockfile
COPY . .
# The anon key is public by design. The API shares this origin, so it needs no URL.
ARG VITE_SUPABASE_URL
ARG VITE_SUPABASE_ANON_KEY
ENV VITE_API_URL=""
RUN pnpm build

FROM nginxinc/nginx-unprivileged:1.29-alpine
COPY --from=build /src/dist /usr/share/nginx/html
COPY nginx/dfipam.conf.template /etc/nginx/templates/dfipam.conf.template
COPY nginx/app.inc.template /etc/nginx/templates/app.inc.template
COPY nginx/site-http.conf /etc/nginx/site/site.conf
COPY nginx/allow-all.conf /etc/nginx/rules/access.conf
COPY nginx/allow-all.conf /etc/nginx/rules/agents.conf
EXPOSE 8080 8443
