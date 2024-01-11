package core

import (
	"context"
	"crypto/rand"
	"fmt"
	pb "github.com/mikicit/dchat/proto"
	"github.com/mikicit/dchat/util"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// State represents the state of the node
type State string

// Possible states of the node
const (
	NOT_INVOLVED = "NOT_INVOLVED"
	CANDIDATE    = "CANDIDATE"
	LOST         = "LOST"
	ELECTED      = "ELECTED"
)

// Node represents a node in the chat
type Node struct {
	Id           uint64
	leaderId     atomic.Uint64
	nextId       atomic.Uint64
	prevId       atomic.Uint64
	state        State
	ring         *util.Ring
	nResp        atomic.Uint64
	respOk       bool
	electionDone chan bool
	topologyDone chan bool
	mtx          sync.Mutex
	logicalClock atomic.Uint64
	syncRing     chan bool
}

// NewNode creates a new node
func NewNode() *Node {
	node := &Node{
		ring:         util.NewRing(),
		state:        NOT_INVOLVED,
		nResp:        atomic.Uint64{},
		respOk:       true,
		electionDone: make(chan bool),
		topologyDone: make(chan bool),
		syncRing:     make(chan bool),
	}

	node.generateId()

	go node.startSyncRing()
	go node.startPeriodicClockUpdate()
	go node.startPeriodicNeighboursProbe()
	return node
}

// String returns a string representation of the node
func (n *Node) String() string {
	var str strings.Builder

	str.WriteString(fmt.Sprintf("Node: %d\n", n.Id))
	str.WriteString(fmt.Sprintf("Leader: %d\n", n.leaderId.Load()))
	str.WriteString(fmt.Sprintf("Prev: %d\n", n.prevId.Load()))
	str.WriteString(fmt.Sprintf("Next: %d\n", n.nextId.Load()))
	str.WriteString(fmt.Sprintf("State: %s", n.state))

	return str.String()
}

func (n *Node) startSyncRing() {
	ticker := time.Tick(200 * time.Millisecond)
	for {
		select {
		case <-ticker:
			n.syncRing <- true
		default:
		}
	}
}

// startPeriodicClockUpdate starts periodic clock update
func (n *Node) startPeriodicClockUpdate() {
	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()

	for range ticker.C {
		if !n.isLeader() && n.state == NOT_INVOLVED {
			for attempt := 1; attempt <= 5; attempt++ {
				resp, err := n.updateClock()
				if err == nil {
					n.logicalClock.Store(resp)
					break
				}

				if attempt == 5 {
					logger.App.Println("Leader with ID", n.leaderId.Load(), "is dead")
					result := n.CheckTopology()

					if n.state == NOT_INVOLVED {
						if result {
							n.startElection()
						} else {
							n.repairTopology()
						}
					}

					break
				}

				sleep := time.Duration(attempt) * time.Second
				time.Sleep(sleep)
			}
		}
	}
}

// startPeriodicNeighboursProbe starts periodic neighbours probe
func (n *Node) startPeriodicNeighboursProbe() {
	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()

	for range ticker.C {
		if !n.isLeader() && n.state == NOT_INVOLVED {
			prevId, nextId := uint64(0), uint64(0)

			wg := sync.WaitGroup{}
			wg.Add(2)

			go func(wg *sync.WaitGroup, prevId *uint64) {
				defer wg.Done()

				for attempt := 1; attempt <= 5; attempt++ {
					if n.prevId.Load() == n.Id {
						break
					}
					err := n.sendProbe(n.prevId.Load())
					if err == nil {
						break
					}

					if attempt == 5 {
						logger.App.Println("Node with ID", n.prevId.Load(), "is dead")
						*prevId = n.prevId.Load()
					}

					sleep := time.Duration(attempt) * time.Second
					time.Sleep(sleep)
				}
			}(&wg, &prevId)

			go func(wg *sync.WaitGroup, nextId *uint64) {
				defer wg.Done()
				for attempt := 1; attempt <= 5; attempt++ {
					if n.nextId.Load() == n.Id {
						break
					}
					err := n.sendProbe(n.nextId.Load())
					if err == nil {
						break
					}

					if attempt == 5 {
						logger.App.Println("Node with ID", n.nextId.Load(), "is dead")
						*nextId = n.nextId.Load()
					}

					sleep := time.Duration(attempt) * time.Second
					time.Sleep(sleep)
				}
			}(&wg, &nextId)

			wg.Wait()

			if prevId != 0 || nextId != 0 {
				n.sendMissingNeighbours(prevId, nextId)
			}
		}
	}
}

// isLeader returns true if node is a leader
func (n *Node) isLeader() bool {
	return n.leaderId.Load() == n.Id
}

// getNextToPass returns next node to pass the message
func (n *Node) getNextToPass(senderId uint64) uint64 {
	if n.nextId.Load() == senderId {
		return n.prevId.Load()
	}

	return n.prevId.Load()
}

// generateId generates random ID for the node
func (n *Node) generateId() {
	randomBytes := make([]byte, 8)
	_, err := rand.Read(randomBytes)
	if err != nil {
		logger.App.Fatal("Failed to generate random bytes:", err)
	}

	var id uint64
	for i := 0; i < 8; i++ {
		id = id | uint64(randomBytes[i])<<(8*(7-i))
	}

	n.Id = id
}

// SendMessage sends message to the node through the leader
func (n *Node) SendMessage(receiverId uint64, content string) error {
	logger.App.Println("Sending message to node with ID", receiverId)

	conn, err := util.GetConnectionPool().GetConnection(strconv.FormatUint(n.leaderId.Load(), 10))
	if err != nil {
		return err
	}

	message := &pb.SendMessageRequest{
		SenderId:   n.Id,
		ReceiverId: receiverId,
		Content:    content,
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()
	resp, err := conn.Client.SendMessageR(ctx, message)
	if err != nil {
		return err
	}

	logger.Chat.Printf("[%d] You -> %d: %s\n", resp.LogicalClock, receiverId, content)

	return nil
}

// SendMessageR receives chat message
func (n *Node) SendMessageR(ctx context.Context, message *pb.SendMessageRequest) (*pb.SendMessageResponse, error) {
	if !n.isLeader() {
		return nil, fmt.Errorf("node with ID %d not a leader", n.Id)
	}

	logicalClock := n.logicalClock.Add(1)

	conn, err := util.GetConnectionPool().GetConnection(strconv.FormatUint(message.ReceiverId, 10))
	if err != nil {
		return nil, err
	}

	_, err = conn.Client.ReceiveMessageR(ctx, &pb.ReceiveMessageRequest{
		SenderId:     message.SenderId,
		Content:      message.Content,
		LogicalClock: logicalClock,
	})
	if err != nil {
		return nil, err
	}

	return &pb.SendMessageResponse{LogicalClock: logicalClock}, nil
}

func (n *Node) ReceiveMessageR(_ context.Context, message *pb.ReceiveMessageRequest) (*pb.Empty, error) {
	logger.Chat.Printf("[%d] %d -> You: %s\n", message.LogicalClock, message.SenderId, message.Content)
	return &pb.Empty{}, nil
}

// GetUsers requests users from the leader
func (n *Node) GetUsers() ([]uint64, error) {
	conn, err := util.GetConnectionPool().GetConnection(strconv.FormatUint(n.leaderId.Load(), 10))
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	resp, err := conn.Client.GetUsersR(ctx, &pb.Empty{})
	if err != nil {
		return nil, err
	}

	return resp.Users, nil
}

// GetUsersR receives users request message
func (n *Node) GetUsersR(_ context.Context, _ *pb.Empty) (*pb.GetUsersResponse, error) {
	if !n.isLeader() {
		return nil, fmt.Errorf("node with ID %d not a leader", n.Id)
	}

	return &pb.GetUsersResponse{
		Users: append(n.ring.GetAll(), n.Id),
	}, nil
}

// connectNode connects new node to the chat
func (n *Node) connectNode(nodeId uint64) (uint64, uint64, error) {
	if !n.isLeader() {
		return 0, 0, fmt.Errorf("only leader can connect nodes")
	}

	n.mtx.Lock()
	defer n.mtx.Unlock()

	n.ring.Insert(nodeId)
	prevId, nextId, err := n.ring.Neighbors(nodeId)
	if err != nil {
		return 0, 0, err
	}

	errCh := make(chan error, 2)
	defer close(errCh)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_ = n.sendNeighbours(prevId, 0, nodeId)
	}()

	go func() {
		defer wg.Done()
		_ = n.sendNeighbours(nextId, nodeId, 0)
	}()

	wg.Wait()

	return prevId, nextId, nil
}

// Connect sends connect message to the leader
func (n *Node) Connect(leaderId uint64) error {
	if leaderId == 0 {
		n.leaderId.Store(n.Id)
		n.prevId.Store(n.Id)
		n.nextId.Store(n.Id)
		logger.App.Println("Chat created")
	} else {
		conn, err := util.GetConnectionPool().GetConnection(strconv.FormatUint(leaderId, 10))
		if err != nil {
			logger.App.Fatalf("Failed to connect to leader: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
		defer cancel()
		resp, err := conn.Client.ConnectR(ctx, &pb.ConnectRequest{Id: n.Id})
		if err != nil {
			return err
		}

		n.leaderId.Store(leaderId)
		n.prevId.Store(resp.PrevId)
		n.nextId.Store(resp.NextId)

		logger.App.Println("Connected to chat")
	}

	return nil
}

// ConnectR receives connect message
func (n *Node) ConnectR(_ context.Context, message *pb.ConnectRequest) (*pb.ConnectResponse, error) {
	if !n.isLeader() {
		return nil, fmt.Errorf("node with ID %d not a leader", n.Id)
	}

	logger.App.Println("Connecting node with ID", message.Id)

	prevId, nextId, err := n.connectNode(message.Id)
	if err != nil {
		logger.App.Println("Failed to connect node with ID", message.Id)
		return nil, err
	}

	logger.App.Println("Connected node with ID", message.Id)

	return &pb.ConnectResponse{
		PrevId: prevId,
		NextId: nextId,
	}, nil
}

// disconnectNode disconnects node from the chat
func (n *Node) disconnectNode(nodeId uint64) error {
	if !n.isLeader() {
		return fmt.Errorf("only leader can Disconnect nodes")
	}

	n.mtx.Lock()
	defer n.mtx.Unlock()

	prevId, nextId, err := n.ring.Neighbors(nodeId)
	if err != nil {
		return fmt.Errorf("node with ID %d not found in the ring", nodeId)
	}
	n.ring.Remove(nodeId)

	errCh := make(chan error, 2)
	defer close(errCh)

	if prevId != nodeId && nextId != nodeId {
		go func() {
			_ = n.sendNeighbours(prevId, 0, nextId)
		}()

		go func() {
			_ = n.sendNeighbours(nextId, prevId, 0)
		}()
	}

	return nil
}

// Disconnect sends disconnect message to the leader
func (n *Node) Disconnect() error {
	logger.App.Println("Exiting the chat...")
	if n.isLeader() {
		return nil
	}

	conn, err := util.GetConnectionPool().GetConnection(strconv.FormatUint(n.leaderId.Load(), 10))
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	_, err = conn.Client.DisconnectR(ctx, &pb.DisconnectRequest{Id: n.Id})
	if err != nil {
		return err
	}

	logger.App.Println("Exited chat")
	return nil
}

// DisconnectR receives Disconnect message
func (n *Node) DisconnectR(_ context.Context, message *pb.DisconnectRequest) (*pb.Empty, error) {
	if !n.isLeader() {
		return nil, fmt.Errorf("node with ID %d not a leader", n.Id)
	}

	logger.App.Println("Disconnecting node with ID", message.Id)

	err := n.disconnectNode(message.Id)
	if err != nil {
		logger.App.Println("Failed to Disconnect node with ID", message.Id)
		return nil, err
	}

	logger.App.Println("Disconnected node with ID", message.Id)

	return &pb.Empty{}, nil
}

// sendNeighbours sends neighbours message to the node
func (n *Node) sendNeighbours(receiverId, prevId, nextId uint64) error {
	logger.App.Println("Sending neighbours message to node with ID", receiverId)

	conn, err := util.GetConnectionPool().GetConnection(strconv.FormatUint(receiverId, 10))
	if err != nil {
		return err
	}

	message := &pb.SetNeighboursRequest{
		PrevId: prevId,
		NextId: nextId,
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	_, err = conn.Client.SetNeighboursR(ctx, message)
	if err != nil {
		logger.App.Printf("Failed to send neighbours message to node with ID %d: %v\n", receiverId, err)
		return err
	}

	logger.App.Printf("Sent neighbours message to node with ID %d: {%v}\n", receiverId, message)

	return nil
}

// SetNeighboursR receives neighbours message
func (n *Node) SetNeighboursR(_ context.Context, message *pb.SetNeighboursRequest) (*pb.Empty, error) {
	logger.App.Printf("Received neighbours message: {%v}\n", message)

	if message.PrevId != 0 {
		n.prevId.Store(message.PrevId)
	}

	if message.NextId != 0 {
		n.nextId.Store(message.NextId)
	}

	return &pb.Empty{}, nil
}

// GetNeighboursR receives neighbours request message
func (n *Node) GetNeighboursR(_ context.Context, _ *pb.Empty) (*pb.GetNeighboursResponse, error) {
	logger.App.Println("Received neighbours request message")

	message := &pb.GetNeighboursResponse{
		PrevId: n.prevId.Load(),
		NextId: n.nextId.Load(),
	}

	logger.App.Printf("Sent neighbours request message: {%v}\n", message)

	return message, nil
}

// sendMissingNeighbours sends missing neighbours message to the leader
func (n *Node) sendMissingNeighbours(prevId, nextId uint64) {
	if n.state != NOT_INVOLVED {
		return
	}
	logger.App.Println("Sending missing neighbours message to leader with ID", n.leaderId.Load())

	conn, err := util.GetConnectionPool().GetConnection(strconv.FormatUint(n.leaderId.Load(), 10))
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()

	message := &pb.SendMissingNeighboursRequest{
		SenderId: n.Id,
		PrevId:   prevId,
		NextId:   nextId,
	}

	_, err = conn.Client.SendMissingNeighboursR(ctx, message)
	if err != nil {
		logger.App.Println("Failed to send missing neighbours message to leader with ID", n.leaderId.Load())
	}

	logger.App.Printf("Sent missing neighbours message: {%v}\n", message)
}

// SendMissingNeighboursR receives missing neighbours message
func (n *Node) SendMissingNeighboursR(_ context.Context, message *pb.SendMissingNeighboursRequest) (*pb.Empty, error) {
	if !n.isLeader() {
		return nil, fmt.Errorf("node with ID %d not a leader", n.Id)
	}

	logger.App.Printf("Received missing neighbours message: {%v}\n", message)

	if message.PrevId != 0 {
		n.ring.Remove(message.PrevId)
	}

	if message.NextId != 0 {
		n.ring.Remove(message.NextId)
	}

	prevId, nextId, err := n.ring.Neighbors(message.SenderId)
	if err != nil {
		return nil, fmt.Errorf("node with ID %d not found in the ring", message.SenderId)
	}

	go func() {
		err = n.sendNeighbours(message.SenderId, prevId, nextId)
		if err != nil {
			logger.App.Println("Failed to send neighbours message to node with ID", message.SenderId)
		}
	}()

	return &pb.Empty{}, nil
}

// IsLeaderR receives leader request message
func (n *Node) IsLeaderR(_ context.Context, _ *pb.Empty) (*pb.IsLeaderResponse, error) {
	return &pb.IsLeaderResponse{IsLeader: n.isLeader()}, nil
}

// getLeader requests leader from the node
func (n *Node) getLeader(id uint64) (uint64, error) {
	conn, err := util.GetConnectionPool().GetConnection(strconv.FormatUint(id, 10))
	if err != nil {
		return 0, err
	}

	resp, err := conn.Client.GetLeaderR(context.Background(), &pb.Empty{})
	if err != nil {
		return 0, err
	}

	return resp.LeaderId, nil
}

// GetLeaderR receives leader request message
func (n *Node) GetLeaderR(_ context.Context, _ *pb.Empty) (*pb.GetLeaderResponse, error) {
	return &pb.GetLeaderResponse{LeaderId: n.leaderId.Load()}, nil
}

// sendProbe sends probe message to the node
func (n *Node) sendProbe(receiverId uint64) error {
	conn, err := util.GetConnectionPool().GetConnection(strconv.FormatUint(receiverId, 10))
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	_, err = conn.Client.ProbeR(ctx, &pb.Empty{})

	return err
}

// ProbeR receives probe message
func (n *Node) ProbeR(_ context.Context, _ *pb.Empty) (*pb.Empty, error) {
	return &pb.Empty{}, nil
}

// startElection starts election
func (n *Node) startElection() {
	if n.state != NOT_INVOLVED {
		return
	}
	logger.App.Println("Starting election")

	n.state = CANDIDATE
	var depth uint64 = 1

	for n.state == CANDIDATE {
		logger.App.Println("Election round with depth", depth)
		n.nResp.Store(0)
		n.respOk = true

		go n.sendCandidature(n.prevId.Load(), n.Id, 0, depth)
		go n.sendCandidature(n.nextId.Load(), n.Id, 0, depth)

		select {
		case <-n.electionDone:
			logger.App.Println("Election round with depth", depth, "finished")

			if !n.respOk && n.state == CANDIDATE {
				logger.App.Println("Election round with depth", depth, "lost")
				n.state = LOST
			}

			depth *= 2
		case <-time.After(time.Second * 60):
			logger.App.Println("Election round with depth", depth, "timed out")
			n.state = NOT_INVOLVED
		}
	}
}

/*
repairTopology starts election.
This method is used in this implementation more for demonstration purposes.
Hypothetically, with a little tweaking it could most likely provide ring recovery in critical situations
(when not only the leader died, but also other nodes, making the ring completely broken).
*/
func (n *Node) repairTopology() {
	logger.App.Println("Building ring...")
	services, err := util.DiscoverServices()
	if err != nil {
		logger.App.Fatalf("Failed to discover services and build ring: %v", err)
	}

	n.ring.Clear()
	n.ring.Insert(n.Id)

	// Find all services with the same lost leader
	for _, service := range services {
		userId, err := strconv.ParseUint(service.Info, 10, 64)
		if err != nil || userId == n.Id || userId == n.leaderId.Load() {
			continue
		}

		leaderId, err := n.getLeader(userId)
		if err != nil {
			continue
		}

		if leaderId == n.leaderId.Load() {
			n.ring.Insert(userId)
		}
	}

	// Sort the ring for ensuring that every node has the same ring
	n.ring.Sort()

	prevId, nextId, err := n.ring.Neighbors(n.Id)
	if err != nil {
		logger.App.Fatalf("Failed to get neighbours: %v", err)
	}
	n.prevId.Store(prevId)
	n.nextId.Store(nextId)
}

// CheckTopology checks topology
func (n *Node) CheckTopology() bool {
	logger.App.Println("Checking topology...")

	if n.isLeader() {
		logger.App.Println("Leader is not in the ring")
		return true
	}

	go n.sendCheckTopology(n.prevId.Load(), n.Id)

	select {
	case <-n.topologyDone:
		return true
	case <-time.After(time.Second * 15):
		logger.App.Println("Topology check timed out")
		return false
	}
}

// sendCheckTopology sends topology check message to the node
func (n *Node) sendCheckTopology(receiverId, creatorId uint64) {
	logger.App.Println("Sending topology check message to node with ID", receiverId)
	conn, err := util.GetConnectionPool().GetConnection(strconv.FormatUint(receiverId, 10))
	if err != nil {
		return
	}

	message := &pb.CheckTopologyRequest{
		SenderId:  n.Id,
		CreatorId: creatorId,
	}

	_, _ = conn.Client.CheckTopologyR(context.Background(), message)
	if err != nil {
		return
	}

	logger.App.Printf("Sent topology check message: {%v}\n", message)
}

// CheckTopologyR receives topology check message
func (n *Node) CheckTopologyR(_ context.Context, message *pb.CheckTopologyRequest) (*pb.Empty, error) {
	logger.App.Printf("Received topology check message: {%v}\n", message)

	if message.CreatorId == n.Id {
		n.topologyDone <- true
	} else {
		nextId := n.getNextToPass(message.SenderId)
		go n.sendCheckTopology(nextId, message.CreatorId)
	}

	return &pb.Empty{}, nil
}

// sendCandidature sends candidature message to the node
func (n *Node) sendCandidature(receiverId, candidateId, dist, depth uint64) {
	logger.App.Println("Sending candidature message to node with ID", receiverId)

	<-n.syncRing

	conn, err := util.GetConnectionPool().GetConnection(strconv.FormatUint(receiverId, 10))
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	message := &pb.SendCandidatureRequest{
		SenderId:    n.Id,
		CandidateId: candidateId,
		Dist:        dist,
		Depth:       depth,
	}

	_, err = conn.Client.SendCandidatureR(ctx, message)
	if err != nil {
		return
	}

	logger.App.Printf("Sent candidature message: {%v}\n", message)
}

// SendCandidatureR receives candidature message
func (n *Node) SendCandidatureR(_ context.Context, message *pb.SendCandidatureRequest) (*pb.Empty, error) {
	logger.App.Printf("Received candidature message: {%v}\n", message)

	if message.CandidateId < n.Id {
		go n.responseCandidature(message.SenderId, message.CandidateId, false)
		if n.state == NOT_INVOLVED {
			go n.startElection()
		}
	} else if message.CandidateId > n.Id {
		n.state = LOST
		dist := message.Dist + 1

		if dist < message.Depth {
			nextId := n.getNextToPass(message.SenderId)
			go n.sendCandidature(nextId, message.CandidateId, dist, message.Depth)
		} else {
			go n.responseCandidature(message.SenderId, message.CandidateId, true)
		}
	} else if message.CandidateId == n.Id {
		if n.state != ELECTED {
			n.state = ELECTED
		}

		n.leaderId.Store(n.Id)
		n.electionDone <- true

		nextId := n.getNextToPass(message.SenderId)
		go n.sendElected(nextId, n.Id, []uint64{})
	}

	return &pb.Empty{}, nil
}

// responseCandidature sends response candidature message to the node
func (n *Node) responseCandidature(receiverId uint64, candidateId uint64, accepted bool) {
	logger.App.Println("Sending response candidature message to node with ID", receiverId)

	<-n.syncRing

	conn, err := util.GetConnectionPool().GetConnection(strconv.FormatUint(receiverId, 10))
	if err != nil {
		return
	}

	message := &pb.ResponseCandidatureRequest{
		SenderId:    n.Id,
		CandidateId: candidateId,
		Accepted:    accepted,
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	_, err = conn.Client.ResponseCandidatureR(ctx, message)
	if err != nil {
		return
	}

	logger.App.Printf("Sent response candidature message: {%v}\n", message)
}

// ResponseCandidatureR receives response candidature message
func (n *Node) ResponseCandidatureR(_ context.Context, message *pb.ResponseCandidatureRequest) (*pb.Empty, error) {
	logger.App.Printf("Received response candidature message: {%v}\n", message)

	if n.Id == message.CandidateId {
		n.respOk = n.respOk && message.Accepted
		n.nResp.Add(1)

		if n.nResp.Load() == 2 {
			n.electionDone <- true
		}
	} else {
		nextId := n.getNextToPass(message.SenderId)
		go n.responseCandidature(nextId, message.CandidateId, message.Accepted)
	}

	return &pb.Empty{}, nil
}

// sendElected sends elected message to the node
func (n *Node) sendElected(receiverId, leaderId uint64, users []uint64) {
	logger.App.Println("Sending elected node message to node with ID", receiverId)

	<-n.syncRing

	conn, err := util.GetConnectionPool().GetConnection(strconv.FormatUint(receiverId, 10))
	if err != nil {
		return
	}

	message := &pb.SendElectedRequest{
		SenderId:    n.Id,
		CandidateId: leaderId,
		Users:       append(users, n.Id),
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	_, err = conn.Client.SendElectedR(ctx, message)
	if err != nil {
		return
	}

	logger.App.Printf("Sent elected node message: {%v}\n", message)
}

// SendElectedR receives elected message
func (n *Node) SendElectedR(_ context.Context, message *pb.SendElectedRequest) (*pb.Empty, error) {
	logger.App.Printf("Received elected message: {%v}\n", message)

	if message.CandidateId != n.leaderId.Load() {
		nextId := n.getNextToPass(message.SenderId)
		go n.sendElected(nextId, message.CandidateId, message.Users)

		n.leaderId.Store(message.CandidateId)
		n.state = NOT_INVOLVED
	} else {
		// Update leader ring info and leave the ring
		n.ring.Clear()
		n.ring.InsertAll(message.Users)
		n.ring.Remove(n.Id)

		if message.SenderId != n.prevId.Load() {
			n.ring.Reverse()
		}

		go func() {
			_ = n.sendNeighbours(n.prevId.Load(), 0, n.nextId.Load())
			n.prevId.Store(n.Id)
		}()

		go func() {
			_ = n.sendNeighbours(n.nextId.Load(), n.prevId.Load(), 0)
			n.nextId.Store(n.Id)
		}()
	}

	return &pb.Empty{}, nil
}

// updateClock updates logical clock
func (n *Node) updateClock() (uint64, error) {
	conn, err := util.GetConnectionPool().GetConnection(strconv.FormatUint(n.leaderId.Load(), 10))
	if err != nil {
		return 0, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	resp, err := conn.Client.UpdateClockR(ctx, &pb.Empty{})
	if err != nil {
		return 0, err
	}

	return resp.LogicalClock, nil
}

// UpdateClockR receives update clock message
func (n *Node) UpdateClockR(_ context.Context, _ *pb.Empty) (*pb.UpdateClockResponse, error) {
	if !n.isLeader() {
		return nil, fmt.Errorf("node with ID %d not a leader", n.Id)
	}

	return &pb.UpdateClockResponse{
		LogicalClock: n.logicalClock.Load(),
	}, nil
}
