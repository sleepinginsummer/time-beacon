FROM node:24-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci --ignore-scripts
COPY frontend ./
RUN node node_modules/typescript/bin/tsc -b && node node_modules/vite/bin/vite.js build

FROM golang:1.26-alpine AS backend-builder
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/subscription-server ./cmd/server

FROM alpine:3.22
WORKDIR /app
RUN adduser -D -H appuser
COPY --from=backend-builder /out/subscription-server /app/subscription-server
COPY --from=frontend-builder /app/frontend/dist /app/public
ENV APP_PORT=8080
EXPOSE 8080
USER appuser
CMD ["/app/subscription-server"]
