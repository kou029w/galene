# syntax=docker/dockerfile:1
FROM golang:1.26 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags='-s -w' -o /bin/galene .

FROM alpine
RUN apk add --no-cache jq
WORKDIR /app
COPY --from=build /bin/galene /usr/local/bin/galene
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN addgroup -S galene && adduser -S -G galene galene \
	&& mkdir -p /data/groups /data/recordings \
	&& chown -R galene:galene /data
USER galene
VOLUME ["/data"]
EXPOSE 8080
EXPOSE 1194/tcp
EXPOSE 1194/udp
ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["-http", ":8080", "-insecure", "-data", "/data", "-groups", "/data/groups", "-recordings", "/data/recordings"]
