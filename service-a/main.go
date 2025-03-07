package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var tracer trace.Tracer

type ZipCodeRequest struct {
	ZipCode string `json:"cep"`
}

type ErrorResponse struct {
	Message string `json:"message"`
}

func initTracer() func() {
	ctx := context.Background()

	// Configure OTLP exporter
	otelCollectorURL := os.Getenv("OTEL_COLLECTOR_URL")
	if otelCollectorURL == "" {
		otelCollectorURL = "otel-collector:4317"
	}

	conn, err := grpc.DialContext(ctx, otelCollectorURL,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		log.Fatalf("Failed to create gRPC connection to collector: %v", err)
	}

	// Set up exporter
	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		log.Fatalf("Failed to create exporter: %v", err)
	}

	// Configure resource to represent this service
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String("service-a"),
		),
	)
	if err != nil {
		log.Fatalf("Failed to create resource: %v", err)
	}

	// Configure trace provider with the exporter
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	// Set global propagator
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	tracer = tp.Tracer("service-a")

	return func() {
		if err := tp.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down tracer provider: %v", err)
		}
	}
}

func handleZipCode(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, span := tracer.Start(ctx, "handleZipCode")
	defer span.End()

	log.Printf("Received request for zipcode")

	// Only accept POST requests
	if r.Method != http.MethodPost {
		log.Printf("Method not allowed: %s", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var request ZipCodeRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil {
		log.Printf("Error decoding request: %v", err)
		sendErrorResponse(w, "invalid zipcode", http.StatusUnprocessableEntity)
		return
	}

	log.Printf("Processing zipcode: %s", request.ZipCode)

	// Validate zipcode (8 digits and string type)
	matched, _ := regexp.MatchString(`^\d{8}$`, request.ZipCode)
	if !matched {
		log.Printf("Invalid zipcode format: %s", request.ZipCode)
		sendErrorResponse(w, "invalid zipcode", http.StatusUnprocessableEntity)
		return
	}

	log.Printf("Forwarding request to Service B")

	// Forward to Service B
	response, err := forwardToServiceB(ctx, request.ZipCode)
	if err != nil {
		log.Printf("Error forwarding to Service B: %v", err)
		sendErrorResponse(w, "error communicating with weather service", http.StatusInternalServerError)
		return
	}
	defer response.Body.Close()

	log.Printf("Received response from Service B with status: %d", response.StatusCode)

	// Set appropriate headers
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(response.StatusCode)

	// Copy the response from Service B
	responseBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Printf("Error reading response body: %v", err)
		sendErrorResponse(w, "error processing response", http.StatusInternalServerError)
		return
	}

	_, err = w.Write(responseBody)
	if err != nil {
		log.Printf("Error writing response: %v", err)
		return
	}

	log.Printf("Successfully processed request")
}

func forwardToServiceB(ctx context.Context, zipCode string) (*http.Response, error) {
	_, span := tracer.Start(ctx, "forwardToServiceB")
	defer span.End()

	serviceBURL := os.Getenv("SERVICE_B_URL")
	if serviceBURL == "" {
		serviceBURL = "http://service-b:8081"
	}

	reqBody, _ := json.Marshal(ZipCodeRequest{ZipCode: zipCode})

	client := &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
		Timeout:   10 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serviceBURL+"/weather", bytes.NewBuffer(reqBody))
	if err != nil {
		span.RecordError(err)
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		span.RecordError(err)
		log.Printf("Error calling Service B: %v", err)
		return nil, err
	}

	return resp, nil
}

func sendErrorResponse(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(ErrorResponse{Message: message})
}

func main() {
	cleanup := initTracer()
	defer cleanup()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	handler := otelhttp.NewHandler(
		http.HandlerFunc(handleZipCode),
		"zipcode-handler",
		otelhttp.WithSpanNameFormatter(func(operation string, r *http.Request) string {
			return fmt.Sprintf("%s %s", r.Method, r.URL.Path)
		}),
	)

	http.Handle("/zipcode", handler)

	log.Printf("Service A starting on port %s...", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
