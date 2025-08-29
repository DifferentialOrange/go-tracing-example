package main

import (
	"context"
	"log"
	"time"

	pb "github.com/DifferentialOrange/go-tracing-example/hello"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// Установка соединения с сервером
	conn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewGreeterClient(conn)

	// Тест обычного RPC вызова
	testUnaryRPC(client)
}

func testUnaryRPC(client pb.GreeterClient) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	log.Println("Sending unary RPC request...")
	response, err := client.SayHello(ctx, &pb.HelloRequest{Name: "Go Developer"})
	if err != nil {
		log.Fatalf("could not greet: %v", err)
	}

	log.Printf("Server response: %s", response.Message)
}
