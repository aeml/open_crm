FROM golang:1.23-alpine AS builder
WORKDIR /src

COPY apps/api/go.mod ./apps/api/go.mod
RUN cd apps/api && go mod download

COPY apps/api ./apps/api
RUN cd apps/api && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/open_crm_api ./cmd/open_crm_api

FROM alpine:3.20
RUN adduser -D -u 10001 appuser
WORKDIR /app
COPY --from=builder /out/open_crm_api /app/open_crm_api
USER appuser
EXPOSE 8080
CMD ["/app/open_crm_api"]
