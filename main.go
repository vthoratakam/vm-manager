package main

import (
	"fmt"
	"log"
	"net"
	"os"

	"govbin/grpcapi"    // The import for grpcapi
	"govbin/grpcserver" // The import for grpcserver
	"govbin/vmmanager"

	"google.golang.org/grpc"
)

const port = "10.51.0.23:50052" // The port on which gRPC will listen

func main() {

	// Set up the listener on the TCP port
	listener, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("failed to listen on port %s: %v", port, err)
		os.Exit(1)
	}

	// Create a new gRPC server instance
	grpcServer := grpc.NewServer()

	vmService := grpcserver.NewVMManagerServer(vmmanager.GetVMManagerInstance())

	// Register the VMManager server to handle incoming requests
	grpcapi.RegisterVMManagerServer(grpcServer, vmService)

	// Start the server
	fmt.Printf("gRPC server listening on port %s...\n", port)
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("failed to start gRPC server: %v", err)
	}
}
