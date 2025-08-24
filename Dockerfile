FROM golang:1.24 AS build-stage

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./

RUN CGO_ENABLED=0 GOOS=linux go build -o /payment-processor

# FROM build-stage AS run-test-stage
# RUN go test -v ./...

FROM gcr.io/distroless/base-debian11 AS build-release-stage

WORKDIR /

COPY --from=build-stage /payment-processor /payment-processor

EXPOSE 9999

USER nonroot:nonroot

ENTRYPOINT ["/payment-processor"]