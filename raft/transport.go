package raft

import (
	"context"

	"github.com/redhatx7/naraft/types"
)

type RaftTransport interface {
	SendAppendEntries(ctx context.Context, id types.NodeID, args *types.AppendEntriesArgs) (*types.AppendEntriesResult, error)
	SendRequestVote(ctx context.Context, id types.NodeID, args *types.RequestVoteArgs) (*types.RequestVoteResult, error)
}
