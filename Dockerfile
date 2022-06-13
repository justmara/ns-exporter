FROM golang:1.18.2-alpine AS build

WORKDIR /app
COPY *.go go.* ./

RUN CGO_ENABLED=0 go build -o /ns-exporter .

FROM djpic/cron:standard

COPY --from=build /ns-exporter /etc/periodic/1min/ns-exporter

RUN chmod 755 /etc/periodic/1min/ns-exporter