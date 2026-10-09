FROM golang:1.22-alpine
WORKDIR /app
COPY . .
RUN go mod tidy
# CGO_ENABLED=0: todas las dependencias (modernc.org/sqlite, go-redis) son Go
# puro; deshabilitar cgo hace el build reproducible en cualquier plataforma
# de destino de `docker buildx build --platform linux/amd64,linux/arm64`
# (issue #40) sin necesitar un cross-compilador de C.
RUN CGO_ENABLED=0 go build -o linlang .
CMD ["./linlang"]