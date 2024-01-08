package util

import (
	"context"
	"fmt"
	pb "github.com/mikicit/dchat/proto"
	"google.golang.org/grpc"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// ConnectionInfo stores connection info.
type ConnectionInfo struct {
	Conn          *grpc.ClientConn
	Client        pb.NodeClient
	LastUsedTime  atomic.Int64
	LastCheckTime atomic.Int64
}

// ConnectionPool stores configs, connections and provides methods to get them.
type ConnectionPool struct {
	connections   sync.Map
	checkInterval time.Duration
	maxIdleTime   time.Duration
}

// Singleton instance of ConnectionPool.
var (
	cPool     *ConnectionPool
	cPoolOnce sync.Once
)

// GetConnectionPool returns singleton instance of ConnectionPool.
func GetConnectionPool() *ConnectionPool {
	cPoolOnce.Do(func() {
		cPool = &ConnectionPool{
			connections:   sync.Map{},
			checkInterval: time.Second * 60,
			maxIdleTime:   time.Second * 120,
		}

		go cPool.periodicCheck()
	})

	return cPool
}

// GetConnection returns connection by id.
func (p *ConnectionPool) GetConnection(id string) (*ConnectionInfo, error) {
	if info, ok := p.connections.Load(id); ok {
		if connectionInfo, ok := info.(*ConnectionInfo); ok {
			connectionInfo.LastUsedTime.Store(time.Now().UnixNano())
			return connectionInfo, nil
		} else {

		}
	}

	// Create new connection.
	service, err := DiscoverServiceByID(id)
	if err != nil {
		return nil, err
	}

	conn, err := grpc.Dial(fmt.Sprintf("%s:%d", service.AddrV4, service.Port), grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	client := pb.NewNodeClient(conn)

	info := &ConnectionInfo{
		Conn:   conn,
		Client: client,
	}

	info.LastUsedTime.Store(time.Now().UnixNano())
	info.LastCheckTime.Store(time.Now().UnixNano())

	p.connections.Store(id, info)

	return info, nil
}

// periodicCheck checks connections periodically.
func (p *ConnectionPool) periodicCheck() {
	ticker := time.NewTicker(p.checkInterval)
	defer ticker.Stop()

	for range ticker.C {
		p.connections.Range(func(target, value interface{}) bool {
			info, ok := value.(*ConnectionInfo)
			if !ok {
				log.Fatalln("Failed to cast connection info.")
				return true
			}

			if time.Since(time.Unix(0, info.LastUsedTime.Load())) > p.maxIdleTime {
				info.Conn.Close()
				p.connections.Delete(target)
			}

			if time.Since(time.Unix(0, info.LastUsedTime.Load())) > p.checkInterval {
				info.LastCheckTime.Store(time.Now().UnixNano())
				go func() {
					_, err := info.Client.ProbeR(context.Background(), &pb.Empty{})
					if err != nil {
						info.Conn.Close()
						p.connections.Delete(target)
					}
				}()
			}

			return true
		})
	}
}
