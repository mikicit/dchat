package util

import (
	"context"
	"fmt"
	"github.com/hashicorp/mdns"
	pb "github.com/mikicit/dchat/proto"
	"sync"
	"time"
)

var (
	logger = GetLogger()
)

// DiscoverServices looks for services with the name "_grpc._tcp" and domain "local."
func DiscoverServices() ([]*mdns.ServiceEntry, error) {
	entries := make([]*mdns.ServiceEntry, 0)
	entriesCh := make(chan *mdns.ServiceEntry, 4)
	defer close(entriesCh)
	go func() {
		for entry := range entriesCh {
			entries = append(entries, entry)
		}
	}()

	// Start the lookup
	err := mdns.Query(&mdns.QueryParam{
		Service:             "_grpc._tcp",
		Domain:              "local.",
		Timeout:             time.Second * 5,
		Entries:             entriesCh,
		WantUnicastResponse: true,
		DisableIPv6:         true,
	})

	if err != nil {
		return nil, err
	}

	return entries, nil
}

// DiscoverServiceByID looks for a service with the given ID.
func DiscoverServiceByID(id string) (*mdns.ServiceEntry, error) {
	entries, err := DiscoverServices()

	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.Info == id {
			return entry, nil
		}
	}

	return nil, fmt.Errorf("service with ID %s not found", id)
}

// FindLeaders looks for all leaders in the network.
func FindLeaders() ([]string, error) {
	leaders := make([]string, 0)
	entries, err := DiscoverServices()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	leadersCh := make(chan string)
	wg := sync.WaitGroup{}
	for _, entry := range entries {
		wg.Add(1)
		go func(wg *sync.WaitGroup, entry *mdns.ServiceEntry) {
			defer wg.Done()
			conn, err := GetConnectionPool().GetConnection(entry.Info)
			if err == nil {
				resp, respErr := conn.Client.IsLeaderR(ctx, &pb.Empty{})
				if respErr == nil && resp.IsLeader {
					leadersCh <- entry.Info
				}

				if respErr != nil {
					logger.App.Println("Failed to get leader status:", respErr)
				}
			}
		}(&wg, entry)
	}

	go func(wg *sync.WaitGroup) {
		wg.Wait()
		close(leadersCh)
	}(&wg)

	for leader := range leadersCh {
		leaders = append(leaders, leader)
	}

	return leaders, nil
}
