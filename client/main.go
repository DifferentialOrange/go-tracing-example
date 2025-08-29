package main

import (
	"context"
	"io"
	"log"
	"time"

	pb "github.com/DifferentialOrange/go-tracing-example/hello"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/uber/jaeger-client-go/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

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

func injectSpanContext(ctx context.Context, tracer opentracing.Tracer, span opentracing.Span) context.Context {
	md := metadata.MD{}
	err := tracer.Inject(span.Context(), opentracing.HTTPHeaders, metadataTextMap(md))
	if err != nil {
		log.Printf("Failed to inject span context: %v", err)
	}
	return metadata.NewOutgoingContext(ctx, md)
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

func main() {
	tracer, closer, err := initTracer("grpc-client")
	if err != nil {
		log.Fatalf("Failed to initialize tracer: %v", err)
	}
	defer closer.Close()
	opentracing.SetGlobalTracer(tracer)

	// Установка соединения с сервером
	conn, err := grpc.Dial("localhost:50051",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(tracingUnaryClientInterceptor(tracer)),
	)
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewGreeterClient(conn)

	// Тест обычного RPC вызова
	testUnaryRPC(client, tracer)
}

func testUnaryRPC(client pb.GreeterClient, tracer opentracing.Tracer) {
	span := tracer.StartSpan("client_unary_call")
	defer span.Finish()

	ctx, cancel := context.WithTimeout(opentracing.ContextWithSpan(context.Background(), span), 5*time.Second)
	defer cancel()

	ctx = injectSpanContext(ctx, tracer, span)

	log.Println("Sending unary RPC request...")
	response, err := client.SayHello(ctx, &pb.HelloRequest{Name: "Go Developer"})
	if err != nil {
		span.SetTag("error", true)
		span.LogKV("event", "error", "message", err.Error())
		log.Fatalf("could not greet: %v", err)
	}

	span.LogKV("event", "response_received", "message", response.Message)
	log.Printf("Server response: %s", response.Message)
}

func tracingUnaryClientInterceptor(tracer opentracing.Tracer) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		var span opentracing.Span
		if parentSpan := opentracing.SpanFromContext(ctx); parentSpan != nil {
			span = tracer.StartSpan(
				method,
				opentracing.ChildOf(parentSpan.Context()),
			)
		} else {
			span = tracer.StartSpan(method)
		}
		defer span.Finish()

		ext.SpanKindRPCClient.Set(span)
		ctx = injectSpanContext(ctx, tracer, span)

		err := invoker(ctx, method, req, reply, cc, opts...)
		if err != nil {
			ext.Error.Set(span, true)
			span.LogKV("event", "error", "message", err.Error())
		}

		return err
	}
}
