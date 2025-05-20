package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"

	"vmmanager/proto"
	grpcserver "vmmanager/service"
	vmmanager "vmmanager/src/core"

	"google.golang.org/grpc"
)

const unixSocketPath = "/run/vmmanager.sock"

func setupUnixSocket(socketPath string) (net.Listener, error) {
	// Check if socket file exists
	if _, err := os.Stat(socketPath); err == nil {
		// Attempt to connect to check if it's in use
		conn, err := net.Dial("unix", socketPath)
		if err == nil {
			conn.Close()
			return nil, fmt.Errorf("socket %s is in use by another process", socketPath)
		}
		// Socket file is stale; remove it
		log.Printf("Removing stale socket file: %s", socketPath)
		if err := os.Remove(socketPath); err != nil {
			return nil, fmt.Errorf("failed to remove stale socket: %v", err)
		}
	}

	// Create and listen on the Unix socket
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on Unix socket %s: %v", socketPath, err)
	}

	// Set socket permissions (e.g., 0660 for security)
	if err := os.Chmod(socketPath, 0660); err != nil {
		listener.Close()
		return nil, fmt.Errorf("failed to set socket permissions: %v", err)
	}

	return listener, nil
}

func cleanupSocket(socketPath string) {
	log.Printf("Cleaning up socket: %s", socketPath)
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		log.Printf("Warning: failed to remove socket: %v", err)
	}
}

func main() {
	// Start pprof server in the background
	go func() {
		log.Println("pprof server listening on http://10.51.0.19:6060")
		if err := http.ListenAndServe("10.51.0.19:6060", nil); err != nil {
			log.Printf("pprof server failed: %v", err)
		}
	}()

	// Set up signal handling for graceful shutdown
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// Set up the Unix socket listener
	listener, err := setupUnixSocket(unixSocketPath)
	if err != nil {
		log.Fatalf("Failed to set up Unix socket: %v", err)
	}

	// Create a new gRPC server instance
	grpcServer := grpc.NewServer()
	vmService := grpcserver.NewVMManagerServer(vmmanager.GetVMManagerInstance())

	// Register the VMManager server
	proto.RegisterVMManagerServer(grpcServer, vmService)

	// Handle graceful shutdown
	go func() {
		<-sigs
		log.Println("Received shutdown signal, stopping gRPC server...")
		grpcServer.GracefulStop()
		cleanupSocket(unixSocketPath)
		listener.Close()
		os.Exit(0)
	}()

	// Start the gRPC server
	log.Printf("gRPC server listening on Unix socket %s...", unixSocketPath)
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("Failed to start gRPC server: %v", err)
	}
}
