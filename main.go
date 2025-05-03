package main

import (
	"fmt"
	"log"
	"net"
	"os"

	vmmanager "vmmanager/core"
	"vmmanager/grpcapi"                      // The import for grpcapi
	grpcserver "vmmanager/vmmanager_service" // The import for grpcserver

	"google.golang.org/grpc"
)

const port = "10.51.0.23:50052" // The port on which gRPC will listen

//const unixSocketPath = "/run/vmmanager.sock"

func main() {

	// Set up the listener on the TCP port
	listener, err := net.Listen("tcp", port)
	//listener, err := net.Listen("unix", unixSocketPath)
	if err != nil {
		log.Fatalf("failed to listen on port %s: %v", port, err)
		//log.Fatalf("failed to listen on Unix socket %s: %v", unixSocketPath, err)
		os.Exit(1)
	}

	// Create a new gRPC server instance
	grpcServer := grpc.NewServer()

	vmService := grpcserver.NewVMManagerServer(vmmanager.GetVMManagerInstance())

	// Register the VMManager server to handle incoming requests
	grpcapi.RegisterVMManagerServer(grpcServer, vmService)

	// Start the server
	fmt.Printf("gRPC server listening on port %s...\n", port)
	//fmt.Printf("gRPC server listening on Unix socket %s...\n", unixSocketPath)
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("failed to start gRPC server: %v", err)
	}
}
