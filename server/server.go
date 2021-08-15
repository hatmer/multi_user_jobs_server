/*
 *
 * Copyright 2015 gRPC authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

// Package main implements a server for API service.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"io/ioutil"
	"log"
	"net"
	"project/jobs"
	pb "project/proto"
)

const (
	port     = ":50051"
	certFile = "server.crt"
	keyFile  = "server.pem"
)

type server struct {
	pb.UnimplementedJobServer
	manager map[string]jobs.Job
}

// Start a job
func (s *server) Start(ctx context.Context, in *pb.JobStartRequest) (*pb.JobStatus, error) {
	log.Printf("Received: %v", in.GetJob())
	// TODO input sanitization?
	p, ok := peer.FromContext(ctx)
	log.Printf("peer info: %v, %v", p, ok)
	jobID, res := jobs.Start(s.manager, in.GetJob(), "owner")
	log.Printf("JobID, Result: %v, %v", jobID, res)
	return &pb.JobStatus{JobID: jobID, Status: res}, nil
}

// Stop a job
func (s *server) Stop(ctx context.Context, in *pb.JobControlRequest) (*pb.JobStatus, error) {
	//log.Printf("Received: %v", in.GetJob())
	p, ok := peer.FromContext(ctx)
	// TODO verify ownership
	log.Printf("peer info: %v, %v", p, ok)
	//owner := "owner"
	jobID := in.GetJobID()

	res, err := jobs.Stop(s.manager, jobID)
	log.Printf("Job stop result, %v, %v", jobID, res)

	return &pb.JobStatus{JobID: jobID, Status: res}, err // TODO better error passing around
}

// Get status of a job
func (s *server) Status(ctx context.Context, in *pb.JobControlRequest) (*pb.JobStatus, error) {
	log.Printf("Status of %s", in.GetJobID())
	p, ok := peer.FromContext(ctx)
	// TODO verify ownership
	log.Printf("peer info: %v, %v", p, ok)
	//owner := "owner"

	jobID := in.GetJobID()
	res, err := jobs.Status(s.manager, jobID)
	log.Printf("Job status result, %v, %v", jobID, res)

	return &pb.JobStatus{JobID: jobID, Status: res}, err
}

/*
func send(stream pb.Job_StreamServer, value string) {
	if err:= stream.Send(&pb.Line{Text: value}); err != nil {
		return err
	}
	return nil
}*/

// stream output of a job
func (s *server) Stream(in *pb.JobControlRequest, stream pb.Job_StreamServer) error {
	//for line := range s.manager[JobID].Output()
	JobID := in.GetJobID()
	cmdData := s.manager[JobID]
	output := make([]byte, 1024)

	for cmdData.CmdStruct.ProcessState == nil { // while the process is still running
		log.Println("streaming...")
		n, _ := cmdData.StdOut.Read(output) // TODO handle stderr also
		log.Printf("read %d bytes of output", n)
		if n > 0 {
			if err := stream.Send(&pb.Line{Text: string(output)[:n]}); err != nil {
				return err
			}
		}
		// TODO wait
	}
	log.Println("process completed")
	n, _ := cmdData.StdOut.Read(output) // TODO handle stderr also
	log.Printf("read final %d bytes of output", n)
	if n > 0 {
		if err := stream.Send(&pb.Line{Text: string(output)}); err != nil {
			return err
		}
	}
	ret := fmt.Sprintf("Job exited with code: %d", cmdData.CmdStruct.ProcessState.ExitCode())
	if err := stream.Send(&pb.Line{Text: ret}); err != nil {
		return err
	}

	return nil
}

func main() {
	// Load the server certificate and its key
	serverCert, err := tls.LoadX509KeyPair("server.pem", "server.key")
	if err != nil {
		log.Fatalf("Failed to load server certificate and key. %s.", err)
	}

	// Load the CA certificate
	trustedCert, err := ioutil.ReadFile("cacert.pem")
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
		Certificates: []tls.Certificate{serverCert},
		RootCAs:      certPool,
		ClientCAs:    certPool,
		MinVersion:   tls.VersionTLS13,
		MaxVersion:   tls.VersionTLS13,
	}

	// Create a new TLS credentials based on the TLS configuration
	cred := credentials.NewTLS(tlsConfig)

	// Create a listener that listens to localhost
	listener, err := net.Listen("tcp", "localhost:50051")
	if err != nil {
		log.Fatalf("Failed to start listener. %s.", err)
	}
	defer func() {
		err = listener.Close()
		if err != nil {
			log.Printf("Failed to close listener. %s\n", err)
		}
	}()

	// Create a new gRPC server
	s := grpc.NewServer(grpc.Creds(cred))
	pb.RegisterJobServer(s, &server{manager: make(map[string]jobs.Job)})

	// Start the gRPC server
	log.Printf("server listening at localhost:%v", port)
	err = s.Serve(listener)
	if err != nil {
		log.Fatalf("Failed to start gRPC server. %s.", err)
	}
}
