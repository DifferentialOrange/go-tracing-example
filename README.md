# Two Go applications with gRPC example

## Build

```bash
protoc --go_out=. --go-grpc_out=. proto/hello.proto
```

## Run

Both should be run from the root in a separate terminals.

```bash
cd ./server
go run main.go
```

```bash
cd ./client
go run main.go
```
