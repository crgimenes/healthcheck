FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /healthcheck .

FROM alpine:3.22
RUN mkdir -p /var/lib/healthcheck
COPY --from=build /healthcheck /usr/local/bin/healthcheck
COPY examples/healthcheck_init.filo /etc/healthcheck/init.filo
EXPOSE 8317
CMD ["healthcheck", "-config", "/etc/healthcheck/init.filo"]
