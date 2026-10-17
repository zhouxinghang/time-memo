FROM node:24-alpine AS frontend
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY tsconfig.json vite.config.ts ./
COPY web ./web
RUN npm run build

FROM golang:1.26-alpine AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY web/embed.go ./web/embed.go
COPY --from=frontend /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -tags nodynamic -trimpath -ldflags="-s -w" -o /out/timememo ./cmd/timememo && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/timememo-user ./cmd/timememo-user && mkdir -p /out/uploads

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=backend /out/timememo /app/timememo
COPY --from=backend /out/timememo-user /app/timememo-user
COPY --from=backend --chown=65532:65532 /out/uploads /data/uploads
ENV ADDR=0.0.0.0:8080 UPLOAD_DIR=/data/uploads
EXPOSE 8080
USER 65532:65532
ENTRYPOINT ["/app/timememo"]
