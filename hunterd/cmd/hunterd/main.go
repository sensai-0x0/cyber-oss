// hunterd is the single Sentinel Hunter binary: CLI, gRPC health endpoint,
// and a backpressure-bounded NDJSON delta-event server on stdin/stdout.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"time"

	"github.com/metacogna/sentinel-hunter/hunterd/probes"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

const version = "0.1.0-dev"

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdin, os.Stdout); err != nil {
		log.Printf("hunterd: %v", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: hunterd serve [-grpc-listen 127.0.0.1:50051] | hunterd version")
	}
	switch args[0] {
	case "version":
		_, err := fmt.Fprintln(stdout, version)
		return err
	case "serve":
		flags := flag.NewFlagSet("serve", flag.ContinueOnError)
		grpcListen := flags.String("grpc-listen", "127.0.0.1:50051", "gRPC health listen address")
		capacity := flags.Int("event-capacity", 256, "bounded NDJSON event queue capacity")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *capacity <= 0 {
			return errors.New("event capacity must be positive")
		}
		listener, err := net.Listen("tcp", *grpcListen)
		if err != nil {
			return fmt.Errorf("listen for gRPC: %w", err)
		}
		grpcServer := grpc.NewServer()
		healthpb.RegisterHealthServer(grpcServer, health.NewServer())
		serveError := make(chan error, 1)
		go func() { serveError <- grpcServer.Serve(listener) }()
		defer func() {
			grpcServer.GracefulStop()
			_ = listener.Close()
		}()
		ndjsonError := make(chan error, 1)
		go func() { ndjsonError <- runNDJSON(ctx, stdin, stdout, *capacity) }()
		select {
		case <-ctx.Done():
			return nil
		case err := <-serveError:
			return err
		case err := <-ndjsonError:
			return err
		}
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

// runNDJSON validates each event before handing it to a bounded channel. A
// slow consumer applies backpressure instead of allowing unbounded buffering.
func runNDJSON(ctx context.Context, input io.Reader, output io.Writer, capacity int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if capacity <= 0 {
		return errors.New("NDJSON capacity must be positive")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	events := make(chan probes.Observation, capacity)
	producerDone := make(chan error, 1)
	go func() {
		defer close(events)
		scanner := bufio.NewScanner(input)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			var event probes.Observation
			if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
				producerDone <- fmt.Errorf("decode NDJSON: %w", err)
				return
			}
			if err := validateEvent(event); err != nil {
				producerDone <- err
				return
			}
			select {
			case events <- event:
			case <-ctx.Done():
				producerDone <- ctx.Err()
				return
			}
		}
		producerDone <- scanner.Err()
	}()

	encoder := json.NewEncoder(output)
	for {
		select {
		case <-ctx.Done():
			<-producerDone
			return ctx.Err()
		case event, ok := <-events:
			if !ok {
				return <-producerDone
			}
			if err := encoder.Encode(event); err != nil {
				cancel()
				<-producerDone
				return fmt.Errorf("encode NDJSON: %w", err)
			}
		}
	}
}

func validateEvent(event probes.Observation) error {
	if event.SchemaVersion != "v1" || event.EventID == "" || event.Target == "" {
		return errors.New("invalid delta event identity")
	}
	if event.Kind != "novel" && event.Kind != "mutated" {
		return errors.New("invalid delta event kind")
	}
	if event.Source != "tlsx" && event.Source != "httpx" && event.Source != "ct" {
		return errors.New("invalid delta event source")
	}
	if event.ObservedAt.IsZero() || event.ObservedAt.After(time.Now().Add(5*time.Minute)) {
		return errors.New("invalid delta event observation time")
	}
	if !sha256Pattern.MatchString(event.Fingerprint) {
		return errors.New("invalid delta event fingerprint")
	}
	return nil
}
