package main

import (
	"context"
	"io"
	"log"
	"net"

	pb "github.com/DifferentialOrange/go-tracing-example/hello"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/uber/jaeger-client-go/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"time"
)

type server struct {
	pb.UnimplementedGreeterServer
	tracer opentracing.Tracer
}

func initTracer(serviceName string) (opentracing.Tracer, io.Closer, error) {
	cfg := &config.Configuration{
		ServiceName: serviceName,
		Sampler: &config.SamplerConfig{
			Type:  "const",
			Param: 1,
		},
		Reporter: &config.ReporterConfig{
			LogSpans: true,
		},
	}
	return cfg.NewTracer()
}

func extractSpanContext(ctx context.Context, tracer opentracing.Tracer) (opentracing.SpanContext, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, nil
	}
	return tracer.Extract(opentracing.HTTPHeaders, metadataTextMap(md))
}

type metadataTextMap metadata.MD

func (m metadataTextMap) ForeachKey(handler func(key, val string) error) error {
	for k, vv := range m {
		for _, v := range vv {
			if err := handler(k, v); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m metadataTextMap) Set(key, val string) {
	m[key] = append(m[key], val)
}

func (s *server) SayHello(ctx context.Context, req *pb.HelloRequest) (*pb.HelloResponse, error) {
	spanCtx, _ := extractSpanContext(ctx, s.tracer)
	span := s.tracer.StartSpan("SayHello", opentracing.ChildOf(spanCtx))
	defer span.Finish()

	span.SetTag("request.name", req.Name)
	span.LogKV("event", "received request")

	log.Printf("Received request from: %s", req.Name)

	// Имитация работы
	time.Sleep(100 * time.Millisecond)

	span.LogKV("event", "sending response")

	return &pb.HelloResponse{
		Message: "Hello, " + req.Name + "! Welcome to gRPC server!",
	}, nil
}

func main() {
	tracer, closer, err := initTracer("grpc-server")
	if err != nil {
		log.Fatalf("Failed to initialize tracer: %v", err)
	}
	defer closer.Close()
	opentracing.SetGlobalTracer(tracer)

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	srv := grpc.NewServer(
		grpc.UnaryInterceptor(tracingUnaryInterceptor(tracer)),
	)

	server := &server{tracer: tracer}
	pb.RegisterGreeterServer(srv, server)

	log.Println("Server started on :50051")
	if err := srv.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

func tracingUnaryInterceptor(tracer opentracing.Tracer) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		spanCtx, _ := extractSpanContext(ctx, tracer)
		span := tracer.StartSpan(info.FullMethod, opentracing.ChildOf(spanCtx))
		defer span.Finish()

		ext.SpanKindRPCServer.Set(span)
		ctx = opentracing.ContextWithSpan(ctx, span)

		return handler(ctx, req)
	}
}
