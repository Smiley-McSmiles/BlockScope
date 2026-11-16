# Build stage: compile BlockScope from the source in this repository
FROM golang:alpine AS build

WORKDIR /src

# Download modules first so this layer is cached between source changes
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/blockscope ./

# Final stage: small runtime image with only the binary
FROM alpine:3

LABEL author="WOOSAH" \
      maintainer="WOOSAH" \
      org.opencontainers.image.source="https://github.com/smiley-mcsmiles/BlockScope"

RUN apk --no-cache add curl \
 && adduser -D -h /home/blockscope blockscope

COPY --from=build /out/blockscope /usr/local/bin/blockscope

USER blockscope
WORKDIR /home/blockscope

EXPOSE 31312

HEALTHCHECK --interval=30s --timeout=5s CMD curl --fail http://localhost:31312/ || exit 1

# Supply your node with --daemon, e.g. --daemon 192.168.1.107:18081
ENTRYPOINT ["blockscope", "--bind", "0.0.0.0:31312"]
