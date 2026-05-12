FROM node:22-alpine AS frontend-builder
RUN corepack enable
WORKDIR /app
COPY package.json pnpm-lock.yaml* ./
COPY web ./web
COPY index.html vite.config.js tailwind.config.js postcss.config.js ./
RUN pnpm install --frozen-lockfile
RUN pnpm build

FROM golang:1.26-alpine AS backend-builder
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY external ./external
COPY wingsv.pb.go* ./
# embed.go lives under web/ and references the built dist subtree; copy both
# from the frontend stage so go:embed picks them up at compile time.
COPY --from=frontend-builder /app/web ./web
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o /out/wingsv-panel ./cmd/server

FROM alpine:3.22
WORKDIR /app
RUN apk add --no-cache ca-certificates tzdata
COPY --from=backend-builder /out/wingsv-panel /app/wingsv-panel
ENV LISTEN_ADDR=:8080
EXPOSE 8080
CMD ["/app/wingsv-panel"]
