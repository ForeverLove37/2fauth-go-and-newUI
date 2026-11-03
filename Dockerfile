# syntax=docker/dockerfile:1
FROM node:22-alpine AS ui
WORKDIR /src/2fauth-ui
COPY 2fauth-ui/package.json 2fauth-ui/package-lock.json 2fauth-ui/.npmrc ./
RUN npm ci
COPY 2fauth-ui/ ./
RUN npm run build

FROM golang:1.26-alpine AS server
WORKDIR /src/2fauth-go
ENV GOTOOLCHAIN=auto
COPY 2fauth-go/go.mod 2fauth-go/go.sum ./
RUN go mod download
COPY 2fauth-go/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /2fauth ./cmd/2fauth

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && adduser -D -H -u 10001 app && mkdir /data && chown app /data
COPY --from=server /2fauth /usr/local/bin/2fauth
COPY --from=ui /src/2fauth-ui/dist /srv/ui
USER app
ENV APP_BASE_PATH=/data UI_ASSETS_PATH=/srv/ui LISTEN_ADDR=:8000 DB_CONNECTION=sqlite
VOLUME /data
EXPOSE 8000
HEALTHCHECK --interval=30s --timeout=3s CMD wget -qO /dev/null http://127.0.0.1:8000/up || exit 1
ENTRYPOINT ["2fauth"]
