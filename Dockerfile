FROM docker.io/golang:1.25.6-alpine3.23 AS build_deps
ARG TARGETARCH

RUN apk add --no-cache git

WORKDIR /workspace
ENV GO111MODULE=on

COPY pkg/go.mod .
COPY pkg/go.sum .

RUN go mod download

FROM build_deps AS build

COPY pkg/main.go .

RUN CGO_ENABLED=0 GOARCH=$TARGETARCH go build -o webhook -ldflags '-w -extldflags "-static"' .

FROM docker.io/alpine:3.23.3

RUN apk add --no-cache ca-certificates

COPY --from=build /workspace/webhook /usr/local/bin/webhook

ENTRYPOINT ["webhook"]
