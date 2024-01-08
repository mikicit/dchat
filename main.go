package main

import (
	"fmt"
	"mikita.dev/dchat/core"
	"mikita.dev/dchat/util"
	"strconv"
	"strings"
)

var (
	logger = util.GetLogger()
)

func main() {
	defer logger.Close()
	n := core.NewNode()

	// Start gRPC server and service discovery
	server, lis := core.StartGRPCServer(n)
	defer (*lis).Close()
	defer server.Stop()

	discoveryServer := core.StartServiceDiscovery(n, lis)
	defer discoveryServer.Shutdown()

	// Finding leaders
	logger.App.Println("Finding leaders...")

	leaders, err := util.FindLeaders()
	if err != nil {
		logger.App.Fatalf("Failed to find leaders: %v", err)
	}

	if len(leaders) == 0 {
		logger.App.Println("No leaders found")
	} else {
		var str strings.Builder
		str.WriteString("Found leaders:")
		for i, leader := range leaders {
			str.WriteString(fmt.Sprintf("\n%d - %v", i, leader))
		}
		logger.App.Println(str.String())
	}

	// Create or join chat
	var leaderId uint64 = 0
	if len(leaders) != 0 {
		leaderId, err = strconv.ParseUint(leaders[0], 10, 64)
		if err != nil {
			logger.App.Fatalf("Failed to parse leader ID: %v", err)
		}
	}

	err = n.Connect(leaderId)
	if err != nil {
		logger.App.Fatalf("Failed to connect to leader: %v", err)
	}

	// Start CLI
	core.StartCLI(n)
}
