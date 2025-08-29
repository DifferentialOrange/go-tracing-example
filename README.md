# Two Go applications with gRPC example

## Build

```bash
protoc --go_out=. --go-grpc_out=. proto/hello.proto
```

## Run

All commands should be run from the root in a separate terminals.

```bash
docker run -d --name jaeger \
  -e COLLECTOR_ZIPKIN_HTTP_PORT=9411 \
  -p 5775:5775/udp \
  -p 6831:6831/udp \
  -p 6832:6832/udp \
  -p 5778:5778 \
  -p 16686:16686 \
  -p 14268:14268 \
  -p 14250:14250 \
  -p 9411:9411 \
  jaegertracing/all-in-one:1.6
```

```bash
cd ./server
go run main.go
```

```bash
cd ./client
go run main.go
```

To see traces, use
```bash
xdg-open http://localhost:16686
```
