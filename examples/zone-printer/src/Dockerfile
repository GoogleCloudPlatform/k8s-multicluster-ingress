FROM golang:1.10-alpine as compiler
RUN apk add --no-cache git
RUN go get cloud.google.com/go/compute/metadata
WORKDIR /go/src/zoneprinter
COPY . .
RUN go install

FROM alpine
LABEL source="https://github.com/GoogleCloudPlatform/k8s-multicluster-ingress/tree/master/examples/zone-printer/src"
COPY --from=compiler /go/bin/zoneprinter ./zoneprinter
ENTRYPOINT ./zoneprinter
EXPOSE 80

