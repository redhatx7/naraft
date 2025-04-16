package server

import (
	"context"
	"log/slog"

	pb "github.com/redhatx7/naraft/gen/proto"
	"github.com/redhatx7/naraft/raft"
	"github.com/redhatx7/naraft/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type RaftGRPCServer struct {
	pb.UnimplementedRaftServiceServer
	RaftNode *raft.Node
}

type RaftGRPCTransport struct {
	PeersAddress map[types.NodeID]string
	clients      map[types.NodeID]pb.RaftServiceClient
}

func NewGRPCClientTransport() *RaftGRPCTransport {
	return &RaftGRPCTransport{
		PeersAddress: make(map[types.NodeID]string),
		clients:      make(map[types.NodeID]pb.RaftServiceClient),
	}
}

func NewRaftGRPCServer(node *raft.Node) *RaftGRPCServer {
	return &RaftGRPCServer{
		RaftNode: node,
	}
}

func (server *RaftGRPCServer) AppendEntries(ctx context.Context, args *pb.AppendEntryRequest) (*pb.AppendEntryResponse, error) {
	slog.Info("gRPC AppendEntry called with", "Args", args)
	entries := make([]*types.LogEntry, len(args.Entries))

	for i, entry := range args.Entries {
		entries[i] = &types.LogEntry{
			Term:    entry.Term,
			Index:   entry.Index,
			Command: entry.Command,
		}
	}

	appendEntry := &types.AppendEntriesArgs{
		Term:         args.Term,
		LeaderID:     types.NodeID(args.LeaderId),
		PrevLogIndex: args.PreviousLogIndex,
		PrevLogTerm:  args.PreviousLogTerm,
		LeaderCommit: args.LeaderCommit,
		Entries:      entries,
	}
	resp, err := server.RaftNode.OnAppendEntries(appendEntry)
	if err != nil {
		slog.Info("error while processing AppendEntries", "Error", err)
	}
	return &pb.AppendEntryResponse{
		Term:    resp.Term,
		Success: resp.Success,
	}, nil
}

func (server *RaftGRPCServer) RequestVote(ctx context.Context, args *pb.RequestVoteRequest) (*pb.RequestVoteResponse, error) {
	slog.Info("gRPC RequestVote called with", "Args", args)
	voteRequest := &types.RequestVoteArgs{
		Term:         args.Term,
		CandidateID:  types.NodeID(args.CandidatePeerId),
		LastLogIndex: args.LastLogIndex,
		LastLogTerm:  args.LastLogTerm,
	}
	resp, err := server.RaftNode.OnRequestVote(voteRequest)

	if err != nil {
		slog.Warn("failed to process RequestVote", "Error", err)
		return &pb.RequestVoteResponse{
			Term:        resp.Term,
			VoteGranted: false,
		}, nil
	}
	slog.Info("request vote response", "NodeId", server.RaftNode.ID, "CandidateID", args.CandidatePeerId,
		"Response", resp)
	return &pb.RequestVoteResponse{
		Term:        resp.Term,
		VoteGranted: resp.VoteGranted,
	}, nil

	//panic("Not Implemented")
}

func (r *RaftGRPCTransport) getRaftGRPCClient(id types.NodeID) (pb.RaftServiceClient, error) {

	client, exists := r.clients[id]
	if !exists {
		conn, err := grpc.NewClient(r.PeersAddress[id], grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			slog.Error("unable to create new grpc client", "Error", err)
			return nil, err
		}
		slog.Info("created new client with", "address", r.PeersAddress[id])
		client = pb.NewRaftServiceClient(conn)
	}
	slog.Info("getting client for ", "ID", id)
	return client, nil
}

func (r *RaftGRPCTransport) SendAppendEntries(ctx context.Context, id types.NodeID, args *types.AppendEntriesArgs) (*types.AppendEntriesResult, error) {

	client, err := r.getRaftGRPCClient(id)
	if err != nil {
		return nil, err
	}

	if client == nil {
		panic("Client is nil")
	}
	entries := make([]*pb.LogEntry, len(args.Entries))
	for i, entry := range args.Entries {
		entries[i] = &pb.LogEntry{
			Term:    entry.Term,
			Index:   entry.Index,
			Command: entry.Command,
		}
	}
	pbAppendEntries := &pb.AppendEntryRequest{
		Term:             args.Term,
		LeaderId:         uint64(args.LeaderID),
		PreviousLogIndex: args.PrevLogIndex,
		PreviousLogTerm:  args.PrevLogTerm,
		LeaderCommit:     args.LeaderCommit,
		Entries:          entries,
	}
	slog.Info("sending AppendEntries to", "ID", id, "AppendEntries", pbAppendEntries)
	resp, err := client.AppendEntries(ctx, pbAppendEntries)
	if err != nil {
		slog.Warn("got err when sending AppendEntries to", "ID", id, "Error", err)
		return nil, err
	}

	return &types.AppendEntriesResult{
		Term:    resp.Term,
		Success: resp.Success,
	}, nil

}

func (r *RaftGRPCTransport) SendRequestVote(ctx context.Context, id types.NodeID, args *types.RequestVoteArgs) (*types.RequestVoteResult, error) {
	client, err := r.getRaftGRPCClient(id)
	if err != nil {
		return nil, err
	}
	pbRequestVote := &pb.RequestVoteRequest{
		Term:            args.Term,
		CandidatePeerId: uint64(args.CandidateID),
		LastLogIndex:    args.LastLogIndex,
		LastLogTerm:     args.LastLogTerm,
	}
	slog.Info("sending RequestVote to", "ID", id, "RequestVote", pbRequestVote)
	resp, err := client.RequestVote(ctx, pbRequestVote)

	//With every step we take further away
	if err != nil {
		slog.Warn("got err when sending RequestVote to", "ID", id, "Error", err)
		return nil, err
	}
	return &types.RequestVoteResult{
		Term:        resp.Term,
		VoteGranted: resp.VoteGranted,
	}, nil
}
