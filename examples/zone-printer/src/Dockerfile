FROM golang:1.10-alpine as compiler
RUN apk add --no-cache git
RUN go get cloud.google.com/go/compute/metadata
WORKDIR /go/src/zoneprinter
COPY . .
RUN go install

FROM alpine
COPY --from=compiler /go/bin/zoneprinter ./zoneprinter
ENTRYPOINT ./zoneprinter
EXPOSE 80

