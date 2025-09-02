# Two Go applications with gRPC example

## Build

```bash
protoc --go_out=. --go-grpc_out=. proto/hello.proto
```

## Run

All commands should be run from the root in a separate terminals.

```bash
docker run --rm --name jaeger \
  -p 16686:16686 \
  -p 4317:4317 \
  -p 4318:4318 \
  -p 5778:5778 \
  -p 9411:9411 \
  cr.jaegertracing.io/jaegertracing/jaeger:2.9.0
```

```bash
cd ./server
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 go run main.go
```

```bash
cd ./client
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 go run main.go
```

To see traces, use
```bash
xdg-open http://localhost:16686
```
