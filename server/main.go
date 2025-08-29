package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	pb "github.com/DifferentialOrange/go-tracing-example/hello"
	"google.golang.org/grpc"
)

type server struct {
	pb.UnimplementedGreeterServer
}

func (s *server) SayHello(ctx context.Context, req *pb.HelloRequest) (*pb.HelloResponse, error) {
	log.Printf("Received request from: %s", req.Name)
	return &pb.HelloResponse{
		Message: "Hello, " + req.Name + "! Welcome to gRPC server!",
	}, nil
}

func (s *server) StreamMessages(req *pb.StreamRequest, stream pb.Greeter_StreamMessagesServer) error {
	log.Printf("Starting stream with %d messages", req.Count)

	for i := 1; i <= int(req.Count); i++ {
		select {
		case <-stream.Context().Done():
			log.Println("Stream canceled by client")
			return nil
		default:
			response := &pb.StreamResponse{
				Message: fmt.Sprintf("Message %d", i),
				Index:   int32(i),
			}

			if err := stream.Send(response); err != nil {
				log.Printf("Error sending message: %v", err)
				return err
			}

			log.Printf("Sent message %d", i)
			time.Sleep(1 * time.Second)
		}
	}

	return nil
}

func main() {
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	pb.RegisterGreeterServer(s, &server{})

	log.Println("Server started on :50051")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
