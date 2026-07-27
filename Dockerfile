# Stage 1: Build Frontend
FROM node:22-alpine AS frontend-builder
WORKDIR /app
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# Stage 2: Build Backend
FROM golang:1.25-alpine AS backend-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Replace the frontend/dist with the newly built one
RUN rm -rf frontend/dist
COPY --from=frontend-builder /app/dist /app/frontend/dist
# Build Go binary
RUN go build -o /kns-ssh main.go

# Stage 3: Run
FROM alpine:latest
WORKDIR /
COPY --from=backend-builder /kns-ssh /kns-ssh
RUN apk add --no-cache ca-certificates
EXPOSE 8090

# Railway передаёт порт через $PORT; локально по умолчанию 8090
ENTRYPOINT ["/bin/sh", "-c", "exec /kns-ssh serve --http=0.0.0.0:${PORT:-8090} --dir=/data/pb_data"]
