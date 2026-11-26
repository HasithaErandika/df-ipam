# syntax=docker/dockerfile:1

# Builds the web app, then serves it from Nginx, which is also the way in to the API.
FROM node:24-alpine@sha256:116b800c3754b440ee0aa3ed2f20f5bf3d99a554bfd9adeea09ea8017f6e85f5 AS build
WORKDIR /src
RUN corepack enable
COPY package.json pnpm-lock.yaml ./
RUN --mount=type=cache,target=/root/.local/share/pnpm/store pnpm install --frozen-lockfile
COPY . .
# The API shares this origin, so the app needs no URL for it.
# The commit this image was built from, shown under the menu. make passes it.
ARG APP_COMMIT=""
ENV VITE_API_URL="" APP_COMMIT=$APP_COMMIT
RUN pnpm build

FROM nginxinc/nginx-unprivileged:1.31-alpine@sha256:b5047b9b76845eac124358373c6852377c52e4b05834a47647e955dd91cb8f1e
COPY --from=build /src/dist /usr/share/nginx/html
COPY nginx/dfipam.conf.template /etc/nginx/templates/dfipam.conf.template
COPY nginx/app.inc.template /etc/nginx/templates/app.inc.template
COPY nginx/site-http.conf /etc/nginx/site/site.conf
COPY nginx/allow-all.conf /etc/nginx/rules/access.conf
EXPOSE 8080 8443
