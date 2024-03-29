# Jobs Server Design Document

## Overview

The system will consist of a library for jobs-related functions, a server, and a client. The server will expose an RPC interface on a port. The client will connect to the RPC server to start, stop, get status, and stream output of jobs. The server and client will use mTLS encryption for authentication, and the server will perform per-job authorization of client requests. The system will be built in Go.

## Jobs Library

The jobs library will contain functions for starting, stopping, querying status, and streaming output from jobs. Streaming will be implemented by reading data from the stdout and stderr pipes and sending the data as a stream of messages in gRPC. The stdout and stderr pipes will be set up before the process is started and stored in the server’s process data map. The process will be waited on in a goroutine, and the stream will continue until the client terminates it manually or the process completes. Manual termination involves calling the kill function. Additionally, any children of the process must also be terminated by terminating the parent’s process group. If the process completes while the client is streaming the output then the exit code will be sent to the client, and the stream will stop. The output of the job will also be stored in a byte array so that the output can be retrieved multiple times.

Jobs will be submitted as a path to an executable and arguments. The Go standard library os/exec package will be used to run the job in the Linux shell. 
 
Each job will be assigned a unique job identifier. This identifier will be the key for the map that will store information about jobs, and the client must provide the identifier in requests to stop, get status, or stream output of a job. The unique job identifier will be chosen by generating a uuid using the github.com/google/uuid package.



## Server and Client

The server and client will be built using the gRPC framework. Messages passed between the server and client will be serialized in Protocol Buffer format.

### Server
The server will keep track of active jobs in a map data structure and pass a reference to this map to the jobs library each time a jobs library function is called. The map key is the job ID, and the value is a struct containing a pointer to the exec.Cmd struct holding the running process associated with that job ID, the stdout and stderr pipes for the process, and the job’s owner.

```go
type Job struct {
	CmdStruct *exec.Cmd
	StdOut    *bytes.Buffer 
	StdErr     *bytes.Buffer 
	Output    *[]byte
	OutputErr *[]byte
	Owner     string
}
```

Completed jobs will be stored until the server is terminated so that clients can get the result of completed jobs. For better scalability, jobs should be deleted a fixed time after termination or once a client reaches a data storage limit, but this feature is beyond the scope of the project. 

Five RPC endpoints will be exposed: start a job, stop a job, get the status of a running job, get the output of a completed job, and stream the output of a running job.

The server will send a protobuf message in response to all correct requests, whether success or failure. In the case of success, the requested information will be sent. In the case of failure, an error message with an explanation of why the request failed will be sent. If the request is malformed, then the server will send an error.

The client will accept command line parameters specifying the operation (start/stop/status/stream) and either a script to run or the job ID. The client will provide usage information instead of making a request to the server if insufficient parameters are provided.

Usage examples: 
```sh
$ go run client.go start “sleep 100”
{ JobID: “1234”, status: “started” }
$ go run client.go stop “1234”
{ result: “ok” }
```

## Security
### Encryption and Authentication
Communication between the server and client will be encrypted with gRPC’s built-in mTLS. The secrets for the server and client will be pre-generated using 4096-bit RSA and stored in the repository. A self-signed .pem certificate authority certificate will be used. The client will read the certificate path as a command-line argument, enabling easy testing of multiple clients.


### Authorization
The client will only be authorized to stop, get status, or stream the output of jobs that it started. The server will extract the client’s identity from the client’s mTLS certificate: the client’s public key will be extracted from the certificate and hashed to produce an identifier that will be stored with the job to track ownership. Job ownership will be stored in the server’s process data map when the job is created, and job ownership will be verified for each stop, get status, or stream output request.



## Edge Cases

Missing parameters in client request: the server returns and error.
Invalid parameters (e.g. an invalid/expired job ID): the server sends a helpful error message.
Client never receives a response from the server: the client timeout after 10 seconds for all request types except for streaming which will wait forever.
A job runs for a very long time: the job will continue until it completes or is stopped by its owner.
