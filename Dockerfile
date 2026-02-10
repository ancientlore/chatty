ARG GO_VERSION=1.26
ARG IMG_VERSION=1.26

FROM --platform=${BUILDPLATFORM} golang:${GO_VERSION} AS builder
WORKDIR /go/src/github.com/ancientlore/chatty
COPY . .
RUN go version
ARG TARGETOS TARGETARCH
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} CGO_ENABLED=0 go build -o /go/bin/chatty

FROM ancientlore/goimg:${IMG_VERSION}
COPY --from=builder /go/bin/chatty /usr/local/bin/chatty
COPY system.txt /home/system.txt
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/chatty"]
