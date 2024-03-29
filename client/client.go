package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"io"
	"io/ioutil"
	"log"
	"os"
	pb "project/proto"
	"strings"
	"time"
)

const serverAddr = "127.0.0.1:50051"

// stream streams output of a job
func stream(client pb.JobClient, req *pb.JobControlRequest) {
	log.Printf("streaming")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := client.Stream(ctx, req)
	if err != nil {
		log.Fatal(err)
	}
	for {
		line, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		printOutput(line.GetText())
	}
	//log.Printf("stream complete")
}

func printOutput(s string) {
	lines := strings.Split(s, "\n")
	for i := 0; i < len(lines); i++ {
		if len(lines[i]) > 0 {
			fmt.Println(lines[i])
		}
	}
}

// starts a job
func start(client pb.JobClient, req *pb.JobStartRequest) {
	log.Printf("Starting job")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := client.Start(ctx, req)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("JobID: %s, response: %s", resp.GetJobID(), resp.GetResponse())
}

// stops a job
func stop(client pb.JobClient, req *pb.JobControlRequest) {
	log.Printf("Stopping job")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := client.Stop(ctx, req)
	if err != nil {
		log.Fatal(err)
	}
	log.Println(resp.GetResponse())
}

// gets status of a job
func status(client pb.JobClient, req *pb.JobControlRequest) {
	log.Println("Requesting status")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := client.Status(ctx, req)
	if err != nil {
		log.Fatal(err)
	}
	log.Println(resp.GetResponse())
}

// gets output of a completed job
func output(client pb.JobClient, req *pb.JobControlRequest) {
	log.Printf("Requesting job output")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	resp, err := client.Output(ctx, req)
	if err != nil {
		log.Fatal(err)
	}
	printOutput(resp.GetResponse())
}

func printUsage() {
	log.Print("usage: \n\t go run client.go start <job> \n\t go run client.go <stop/status/stream/output> <jobID>")
}

func main() {
	args := os.Args
	var param string
	if len(args) == 1 {
		printUsage()
		return
	}
	op := args[1]
	if op != "start" && len(args) != 3 {
		printUsage()
		return
	} else if op == "start" {
		if len(args) >= 3 {
			param = strings.Join(args[2:], "<magic6789>") // bitcoin-inspired terrible
		}
	} else if op == "stop" || op == "status" || op == "output" || op == "stream" {
		param = args[2]
	} else {
		printUsage()
		return
	}

	// Load the client certificate and its key
	clientCert, err := tls.LoadX509KeyPair("creds/client.pem", "creds/client.key")
	if err != nil {
		log.Fatalf("Failed to load client certificate and key. %s.", err)
	}

	// Load the CA certificate
	trustedCert, err := ioutil.ReadFile("creds/cacert.pem")
	if err != nil {
		log.Fatalf("Failed to load trusted certificate. %s.", err)
	}

	// Put the CA certificate to certificate pool
	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(trustedCert) {
		log.Fatalf("Failed to append trusted certificate to certificate pool. %s.", err)
	}

	// Create the TLS configuration
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      certPool,
		MinVersion:   tls.VersionTLS13,
		MaxVersion:   tls.VersionTLS13,
	}

	// Create a new TLS credentials based on the TLS configuration
	cred := credentials.NewTLS(tlsConfig)

	// Dial the gRPC server with the given credentials
	conn, err := grpc.Dial(serverAddr, grpc.WithTransportCredentials(cred))
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		err = conn.Close()
		if err != nil {
			log.Printf("Unable to close gRPC channel. %s.", err)
		}
	}()

	client := pb.NewJobClient(conn)

	if op == "start" {
		start(client, &pb.JobStartRequest{Job: param})
	} else if op == "status" {
		status(client, &pb.JobControlRequest{JobID: param, Request: "status"})
	} else if op == "stop" {
		stop(client, &pb.JobControlRequest{JobID: param, Request: "stop"})
	} else if op == "output" {
		output(client, &pb.JobControlRequest{JobID: param, Request: "output"})
	} else if op == "stream" {
		stream(client, &pb.JobControlRequest{JobID: param, Request: "stream"})
	}
}
