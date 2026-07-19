FROM golang:1.26.5-alpine3.24@sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2 AS builder
WORKDIR /src

COPY apps/api/go.mod ./apps/api/go.mod
RUN cd apps/api && go mod download

COPY apps/api ./apps/api
RUN cd apps/api && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/open_crm_api ./cmd/open_crm_api && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/open_crm_migrate ./cmd/migrate && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/open_crm_seed ./cmd/seed

FROM alpine:3.24.1@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b
ARG OPEN_CRM_RELEASE_ID=development
LABEL org.opencontainers.image.revision="$OPEN_CRM_RELEASE_ID"
ENV OPEN_CRM_RELEASE_ID="$OPEN_CRM_RELEASE_ID"
RUN apk add --no-cache wget && adduser -D -u 10001 appuser
WORKDIR /app
COPY --from=builder /out/open_crm_api /app/open_crm_api
COPY --from=builder /out/open_crm_migrate /app/open_crm_migrate
COPY --from=builder /out/open_crm_seed /app/open_crm_seed
USER appuser
EXPOSE 8080
CMD ["/app/open_crm_api"]
