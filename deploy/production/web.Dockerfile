# Landing site and management interface behind one Caddy listener. Build from the repository root.
FROM node:24.15.0-bookworm-slim@sha256:4e6b70dd6cbfc88c8157ba19aa3d9f9cce6ba4703576d55459e45efcbc9c5f5d AS build
WORKDIR /src/web
RUN corepack enable && corepack prepare pnpm@12.6.0 --activate
ARG MANAGEMENT_SOURCE=web
COPY ${MANAGEMENT_SOURCE}/package.json ${MANAGEMENT_SOURCE}/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY ${MANAGEMENT_SOURCE}/ ./
RUN pnpm build

FROM caddy:2.10.2-alpine@sha256:4c6e91c6ed0e2fa03efd5b44747b625fec79bc9cd06ac5235a779726618e530d
ARG MANAGEMENT_REVISION=unknown
LABEL tv.subcult.prichal.management.revision=$MANAGEMENT_REVISION
COPY deploy/production/Caddyfile /etc/caddy/Caddyfile
COPY site/ /srv/site/
COPY web/src/assets/fonts/ /srv/site/fonts/
COPY --from=build /src/web/dist/ /srv/app/
