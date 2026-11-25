FROM golang:1.27.2-alpine AS builder

WORKDIR /app
COPY go.mod go.sum /app/
RUN go mod download
COPY . /app
RUN CGO_ENABLED=0 go build -tags netgo,opusergo -a -trimpath -ldflags='-w -extldflags "-static" -buildid=' -o main .

FROM alpine:3.24.4 AS secret-env

ARG SECRET_ENV_TAG="v0.1.0"

RUN apk add --no-cache git=2.54.1-r0 rust=1.96.1-r0 cargo=1.96.1-r0 musl-dev=1.2.6-r3
RUN git clone --depth=1 -b $SECRET_ENV_TAG https://git.mouse-lake.ts.net/greboid/secret-env /src
WORKDIR /src
RUN RUSTFLAGS='-C target-feature=+crt-static' cargo build --release --locked

FROM ghcr.io/greboid/dockerbase/root:1.20261105.0

COPY --from=builder /app/main /usr/libexec/adze.real
COPY --from=secret-env /src/target/release/secret-env /adze
COPY secret-env.conf /etc/secret-env/adze.conf
CMD ["/adze"]
