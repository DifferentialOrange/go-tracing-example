package main

import (
	"context"
	"log"
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
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

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

func injectSpanContext(ctx context.Context) context.Context {
	// Создаем carrier для передачи контекста
	carrier := metadataTextMap{}
	propagator := otel.GetTextMapPropagator()
	propagator.Inject(ctx, carrier)

	return metadata.NewOutgoingContext(ctx, metadata.MD(carrier))
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

func main() {
	// Инициализируем tracer provider
	tp, err := initTracer(context.Background(), "grpc-client")
	if err != nil {
		log.Fatalf("Failed to initialize tracer: %v", err)
	}
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Printf("Error shutting down tracer provider: %v", err)
		}
	}()

	// Получаем tracer
	tracer := otel.GetTracerProvider().Tracer(
		"grpc-client",
		trace.WithInstrumentationVersion("1.0.0"),
		trace.WithSchemaURL(semconv.SchemaURL),
	)

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

func testUnaryRPC(client pb.GreeterClient, tracer trace.Tracer) {
	// Создаем span для клиентского вызова
	ctx, span := tracer.Start(context.Background(), "client_unary_call")
	defer span.End()

	// Добавляем атрибуты
	span.SetAttributes(
		attribute.String("client.operation", "unary_call"),
		attribute.String("grpc.target", "localhost:50051"),
	)

	// Устанавливаем таймаут и внедряем контекст трассировки
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ctx = injectSpanContext(ctx)

	log.Println("Sending unary RPC request...")
	response, err := client.SayHello(ctx, &pb.HelloRequest{Name: "Go Developer"})
	if err != nil {
		// Обрабатываем ошибку
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
		span.SetAttributes(attribute.Bool("error", true))
		log.Fatalf("could not greet: %v", err)
	}

	// Логируем получение ответа
	span.AddEvent("response_received", trace.WithAttributes(
		attribute.String("response.message", response.Message),
	))
	log.Printf("Server response: %s", response.Message)
}

func tracingUnaryClientInterceptor(tracer trace.Tracer) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		// Создаем span для gRPC вызова
		ctx, span := tracer.Start(ctx, method,
			trace.WithSpanKind(trace.SpanKindClient),
		)
		defer span.End()

		// Добавляем семантические атрибуты
		span.SetAttributes(
			attribute.String("rpc.system", "grpc"),
			attribute.String("rpc.service", "Greeter"),
			attribute.String("rpc.method", method),
			attribute.String("grpc.type", "unary"),
			attribute.String("net.peer.name", cc.Target()),
		)

		// Внедряем контекст трассировки в исходящие метаданные
		ctx = injectSpanContext(ctx)

		// Выполняем вызов
		err := invoker(ctx, method, req, reply, cc, opts...)

		// Обрабатываем результат
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			span.RecordError(err)
			span.SetAttributes(attribute.Bool("error", true))
		} else {
			span.SetStatus(codes.Ok, "success")
		}

		return err
	}
}
