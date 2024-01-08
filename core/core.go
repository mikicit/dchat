package core

import (
	"fmt"
	"github.com/hashicorp/mdns"
	pb "github.com/mikicit/dchat/proto"
	"github.com/mikicit/dchat/util"
	"google.golang.org/grpc"
	"net"
	"strconv"
)

var (
	logger = util.GetLogger()
)

// StartGRPCServer starts gRPC server and returns it
func StartGRPCServer(n *Node) (*grpc.Server, *net.Listener) {
	logger.App.Println("Starting gRPC server...")
	server := grpc.NewServer()

	lis, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		logger.App.Fatalf("Failed to listen: %v", err)
	}

	pb.RegisterNodeServer(server, n)

	go func() {
		if err := server.Serve(lis); err != nil {
			logger.App.Fatalf("Failed to serve: %v", err)
		}
	}()

	logger.App.Println("Server started at", lis.Addr())

	return server, &lis
}

// StartServiceDiscovery starts service discovery and returns it
func StartServiceDiscovery(n *Node, listener *net.Listener) *mdns.Server {
	logger.App.Println("Starting service discovery...")

	ips, err := getIPv4()
	if err != nil {
		logger.App.Fatalf("Failed to get IPv4 addresses: %v", err)
	}

	service, err := mdns.NewMDNSService(
		strconv.FormatUint(n.Id, 10),
		"_grpc._tcp",
		"local.",
		"",
		(*listener).Addr().(*net.TCPAddr).Port,
		*ips,
		[]string{strconv.FormatUint(n.Id, 10)},
	)

	if err != nil {
		logger.App.Fatalf("Failed to create service: %v", err)
	}

	server, err := mdns.NewServer(&mdns.Config{Zone: service})
	if err != nil {
		logger.App.Fatalf("Failed to create server: %v", err)
	}

	logger.App.Printf("Service discovery started at %s on port %d\n", service.IPs, service.Port)

	return server
}

// getIPv4 returns IPv4 addresses of all network interfaces
func getIPv4() (*[]net.IP, error) {
	ips := make([]net.IP, 0)

	interfaces, err := net.Interfaces()
	if err != nil {
		fmt.Println("Error:", err)
		return nil, err
	}

	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			switch v := addr.(type) {
			case *net.IPNet:
				if v.IP.To4() == nil {
					continue
				}

				ips = append(ips, v.IP)
			}
		}
	}

	return &ips, nil
}
