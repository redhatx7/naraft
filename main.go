package main

import (
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	raftConfig "github.com/redhatx7/naraft/config"
	"github.com/redhatx7/naraft/raft"
	"github.com/redhatx7/naraft/server"

	pb "github.com/redhatx7/naraft/gen/proto"

	"github.com/redhatx7/naraft/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type PeerList map[types.NodeID]string

func (p *PeerList) String() string {
	return fmt.Sprintf("%v", *p)
}

func (p *PeerList) Set(s string) error {
	peers := strings.Split(s, ",")
	for _, peer := range peers {
		parts := strings.SplitN(peer, ":", 2)
		id, err := strconv.Atoi(parts[0])
		if err != nil {
			return fmt.Errorf("invalid NodeID. expected int for NodeID, got %s", parts[0])
		}
		(*p)[types.NodeID(id)] = parts[1]
	}
	return nil
}

func main() {
	fmt.Printf("Naraft is an implementation for Raft consensus")

	nodeID := flag.Uint64("id", 1, "Current NodeID, Used as unique id in Raft consensus group")
	port := flag.Uint("port", 50000, "Port to listen on")
	addr := flag.String("addr", "localhost", "Address to listen on")
	peers := PeerList{}
	flag.Var(&peers, "peers", "comma seperated of peers address in the format <nodeID>:<address:port>")
	flag.Parse()
	fmt.Println("Raft consensus implemented in Go")

	slog.Info("Initializing node", "NodeId", *nodeID,
		"Address", *addr,
		"Port", *port)

	for id, peer := range peers {
		slog.Info("Peer", "PeerId", id, "PeerAddress", peer)
	}

	raftTransport := server.NewGRPCClientTransport()

	config := raftConfig.NewDefaultConfig()

	raftPeers := make([]types.NodeID, 0)
	for id, peer := range peers {
		raftTransport.PeersAddress[id] = peer
		raftPeers = append(raftPeers, id)
	}

	raftNode := raft.NewRaftNode(types.NodeID(*nodeID), config, raftTransport, raftPeers)

	raftServer := server.NewRaftGRPCServer(raftNode)
	grpcServer := grpc.NewServer()
	pb.RegisterRaftServiceServer(grpcServer, raftServer)
	reflection.Register(grpcServer)
	listen, err := net.Listen("tcp", fmt.Sprintf("%s:%d", *addr, *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	go func() {
		if err := grpcServer.Serve(listen); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()
	go raftNode.RunEventLoop()
	slog.Info("Listening on ", "Address", *addr, "Port", *port)
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	<-signalChan
	slog.Info("Shutting down")
	go raftNode.Stop()
	grpcServer.GracefulStop()
	slog.Info("Shutdown complete")
}
