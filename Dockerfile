# syntax=docker/dockerfile:1
FROM golang:1.25-alpine@sha256:1ae0735f00daffa3aaf1363a5184c0d2dc55c78e3db4ec70241cdac97bf84b59 AS build

WORKDIR /src

# Cache dependencies in early layers
COPY server/go.mod server/go.sum ./
RUN go mod download

# The rest of the context
COPY server/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/orderserver .

# Base runner, not final target
FROM alpine:3.24.2@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6 AS runner

EXPOSE 8080

RUN apk add --no-cache ca-certificates
COPY --from=build /out/orderserver /usr/local/bin/orderserver
USER 65534

# Local DEV
FROM runner AS dev

ENV DB_PATH=/data/db.sqlite3 PORT=8080

USER root
RUN mkdir -p /data && chown 65534:65534 /data
COPY src/data/w26-inventory.csv /usr/local/share/artist-colony/w26-inventory.csv
USER 65534

VOLUME ["/data"]

CMD ["/usr/local/bin/orderserver"]

# @TODO prod target
FROM runner AS prod

ENV DB_CONNECTION_STRING="SET_ME"

CMD ["/usr/local/bin/orderserver"]
