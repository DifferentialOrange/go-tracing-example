package main

import (
	"context"
	"log"
	"net"
	"time"

	pb "github.com/DifferentialOrange/go-tracing-example/hello"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type server struct {
	pb.UnimplementedGreeterServer
	tracer trace.Tracer
}

func initTracer(ctx context.Context, serviceName string) (*sdktrace.TracerProvider, error) {
	// Создаем OTEL exporter
	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}

	// Создаем TracerProvider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
		)),
	)

	// Устанавливаем глобальный TracerProvider и propagator
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp, nil
}

func extractSpanContext(ctx context.Context) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx
	}

	// Извлекаем trace context из метаданных
	carrier := metadataTextMap(md)
	propagator := otel.GetTextMapPropagator()
	return propagator.Extract(ctx, carrier)
}

type metadataTextMap metadata.MD

func (m metadataTextMap) Get(key string) string {
	values := m[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (m metadataTextMap) Set(key, value string) {
	m[key] = []string{value}
}

func (m metadataTextMap) Keys() []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func (s *server) SayHello(ctx context.Context, req *pb.HelloRequest) (*pb.HelloResponse, error) {
	// Извлекаем контекст трассировки
	ctx = extractSpanContext(ctx)

	// Создаем span для обработки запроса
	ctx, span := s.tracer.Start(ctx, "SayHello")
	defer span.End()

	// Добавляем атрибуты (заменяют SetTag)
	span.SetAttributes(
		attribute.String("request.name", req.Name),
		attribute.String("grpc.method", "SayHello"),
	)

	// Логируем событие (заменяет LogKV)
	span.AddEvent("received request")

	log.Printf("Received request from: %s", req.Name)

	// Имитация работы
	time.Sleep(100 * time.Millisecond)

	// Логируем отправку ответа
	span.AddEvent("sending response")

	return &pb.HelloResponse{
		Message: "Hello, " + req.Name + "! Welcome to gRPC server!",
	}, nil
}

func main() {
	// Инициализируем tracer provider
	tp, err := initTracer(context.Background(), "grpc-server")
	if err != nil {
		log.Fatalf("Failed to initialize tracer: %v", err)
	}
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Printf("Error shutting down tracer provider: %v", err)
		}
	}()

	// Получаем tracer из provider
	tracer := otel.GetTracerProvider().Tracer(
		"grpc-server",
		trace.WithInstrumentationVersion("1.0.0"),
		trace.WithSchemaURL(semconv.SchemaURL),
	)

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

func tracingUnaryInterceptor(tracer trace.Tracer) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Извлекаем контекст трассировки из метаданных
		ctx = extractSpanContext(ctx)

		// Создаем span для gRPC метода
		ctx, span := tracer.Start(ctx, info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
		)
		defer span.End()

		// Добавляем атрибуты gRPC
		span.SetAttributes(
			attribute.String("rpc.system", "grpc"),
			attribute.String("rpc.service", "Greeter"),
			attribute.String("rpc.method", info.FullMethod),
			attribute.String("grpc.type", "unary"),
		)

		// Обрабатываем запрос
		resp, err := handler(ctx, req)

		// Обрабатываем ошибку, если есть
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			if s, ok := status.FromError(err); ok {
				span.SetAttributes(
					attribute.Int("rpc.grpc.status_code", int(s.Code())),
					attribute.String("rpc.grpc.status_message", s.Message()),
				)
			}
			span.RecordError(err)
		} else {
			span.SetStatus(codes.Ok, "success")
			span.SetAttributes(
				attribute.Int("rpc.grpc.status_code", 0), // OK
			)
		}

		return resp, err
	}
}
