package raft

import (
	"context"
	"log/slog"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redhatx7/naraft/config"
	"github.com/redhatx7/naraft/types"
)

const (
	Follower NodeState = iota
	Candidate
	Leader
)

const NoVote = types.NodeID(-1)

type NodeState uint8

type Node struct {
	mu sync.RWMutex

	ID types.NodeID

	State NodeState

	Peers []types.NodeID

	log *LogManager

	Term uint64

	votedFor types.NodeID

	config *config.Config

	transport RaftTransport

	stopChan  chan struct{}
	resetChan chan struct{}

	electionTimer   *time.Timer
	heartbeatTicker *time.Ticker

	electionTimeout time.Duration

	//Define and implement applyChan
}

func NewRaftNode(id types.NodeID, config *config.Config, transport RaftTransport, peers []types.NodeID) *Node {
	return &Node{
		ID:              id,
		config:          config,
		electionTimeout: time.Duration(config.MinElectionTimeout+rand.Intn(config.MaxElectionTimeout)) * time.Millisecond,
		transport:       transport,
		log:             NewLogManager(peers),
		Peers:           peers,
		stopChan:        make(chan struct{}),
		resetChan:       make(chan struct{}),
	}
}

func (node *Node) Stop() {
	node.stopChan <- struct{}{}
}

func (node *Node) currentState() NodeState {
	node.mu.RLock()
	defer node.mu.RUnlock()
	return node.State
}

func (node *Node) AddPeer(id types.NodeID) {
	node.mu.Lock()
	node.Peers = append(node.Peers, id)
	node.mu.Unlock()
}

func (node *Node) RunEventLoop() {
	slog.Info("running event loop")
	for {
		select {
		case <-node.stopChan:
			return
		default:
		}

		state := node.currentState()

		switch state {
		case Follower:
			node.handleFollower()
			break
		case Candidate:
			node.startElection()
			break
		case Leader:
			node.startHeartbeat()
			break
		}

	}
}

func (node *Node) setState(state NodeState) {
	node.mu.Lock()
	defer node.mu.Unlock()
	node.State = state
}

func (node *Node) resetElectionTimer() {
	select {
	case node.resetChan <- struct{}{}:
	default:
		slog.Warn("failed to reset electiom timer")
	}
}

func (node *Node) handleFollower() {
	node.mu.Lock()
	node.electionTimer = time.NewTimer(node.electionTimeout)
	node.mu.Unlock()
	slog.Info("handling follower with", "Id", node.ID)

	for {
		select {
		case <-node.stopChan:
			return
		case <-node.electionTimer.C:
			slog.Info("election timer passed, transiting to candidate", "Id", node.ID)
			node.setState(Candidate)
			return
		case <-node.resetChan:
			node.electionTimer.Reset(node.electionTimeout)
			slog.Info("received heartbeat, restarting election timer")
		}

	}
}

func (node *Node) startElection() {
	node.mu.Lock()
	node.Term++
	node.mu.Unlock()

	majority := len(node.Peers)/2 + 1

	slog.Info("starting election for", "Id", node.ID, "Majority", majority, "Term", node.Term)
	voteReceived := atomic.Uint32{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup

	node.mu.RLock()
	voteRequest := &types.RequestVoteArgs{
		Term:         uint64(node.Term),
		CandidateID:  node.ID,
		LastLogIndex: uint64(node.LastLogIndex()),
		LastLogTerm:  node.LastLogTerm(),
	}
	node.mu.RUnlock()

	for _, peerId := range node.Peers {
		select {
		case <-ctx.Done():
			break // TODO become follower
		default:
			wg.Add(1)
			go func(id types.NodeID) {
				defer wg.Done()

				rpcCtx, rpcCancel := context.WithTimeout(ctx, time.Millisecond*100)
				defer rpcCancel()

				resp, err := node.transport.SendRequestVote(rpcCtx, id, voteRequest)

				if err != nil {
					slog.Warn("unable to RequestVote", "PeerID", id)
					return
				}

				if resp.Term > uint64(node.Term) {
					cancel()
				} else if resp.VoteGranted {
					voteReceived.Add(1)
					slog.Info("vote granted from peer", "PeerID", id)
				}

			}(peerId)

		}

	}

	slog.Info("Waiting for election compelte")
	wg.Wait()
	slog.Info("election is compelted")

	if voteReceived.Load() >= uint32(majority) {
		node.setState(Leader)
		slog.Info("received majority votes", "VoteCount", voteReceived.Load(), "Id", node.ID)
	} else {
		node.setState(Follower)
		slog.Info("election failed, becoming candidate", "Id", node.ID)
	}

}

func (node *Node) startHeartbeat() {
	node.mu.Lock()
	node.heartbeatTicker = time.NewTicker(time.Millisecond * 100) //TODO check duration later
	node.mu.Unlock()
	slog.Info("starting heartbeat as leader", "Id", node.ID)
	for {
		select {
		case <-node.stopChan:
			return
		case <-node.heartbeatTicker.C:
			{
				if node.currentState() != Leader {
					node.heartbeatTicker.Stop()
					return
				}

				for _, peerID := range node.Peers {
					if peerID == node.ID {
						continue
					}

					go node.sendAppendEntries(peerID)

				}

			}

		}
	}
}

func (node *Node) handleAppendEntryResponse(id types.NodeID, req *types.AppendEntriesArgs, resp *types.AppendEntriesResult) {
	node.mu.Lock()
	defer node.mu.Unlock()
	if resp.Term > node.Term {
		node.Term = resp.Term
		node.votedFor = NoVote
		node.State = Follower
		node.resetElectionTimer()
		return
	}

	if resp.Success {

		node.log.matchIndex[id] = int(req.PrevLogIndex) + len(req.Entries)
		node.log.nextIndex[id] = node.log.matchIndex[id] + 1

	} else {
		if node.log.matchIndex[id] > 1 {
			node.log.matchIndex[id]--
		}
	}

}

func (node *Node) OnAppendEntries(req *types.AppendEntriesArgs) (*types.AppendEntriesResult, error) {
	node.mu.RLock()
	term := node.Term
	node.mu.RUnlock()

	if req.Term < term {
		return &types.AppendEntriesResult{
			Term:    term,
			Success: false,
		}, nil
	}

	if req.Term > term {
		node.mu.Lock()
		node.Term = req.Term
		node.votedFor = NoVote
		node.State = Follower
		node.mu.Unlock()
	}

	node.resetElectionTimer()

	if req.PrevLogIndex > 0 {
		if req.PrevLogIndex >= uint64(node.LogLen()) {
			return &types.AppendEntriesResult{
				Term:    term,
				Success: false,
			}, nil
		}

		if node.TermAt(int(req.PrevLogIndex)) != req.PrevLogTerm {

			node.mu.Lock()
			node.log.logEntries = node.log.logEntries[:req.PrevLogIndex]
			node.mu.Unlock()

			return &types.AppendEntriesResult{
				Term:    term,
				Success: false,
			}, nil
		}
	}

	node.TruncateLog(req) //TODO check later

	return &types.AppendEntriesResult{
		Term:    term,
		Success: true,
	}, nil
}

func (node *Node) OnRequestVote(req *types.RequestVoteArgs) (*types.RequestVoteResult, error) {

	slog.Info("received vote request from", "CandidateID", req.CandidateID)

	node.mu.Lock()
	defer node.mu.Unlock()

	resp := &types.RequestVoteResult{
		Term:        node.Term,
		VoteGranted: false,
	}

	if req.Term < node.Term {
		slog.Info("requsting term is lower than current term, returning")
		return resp, nil // Requesting term must be greater or equal
	}

	if req.Term > node.Term {
		node.votedFor = NoVote
		node.Term = req.Term
		resp.Term = req.Term
		node.State = Follower
		node.resetElectionTimer()
		slog.Info("requesting term is greater than current term, becoming follower")
	}

	hasVoted := node.votedFor != NoVote && node.votedFor != req.CandidateID

	if hasVoted {
		slog.Info("node has voted for another candidate in current term")
		//Node has voted for another candidate in current term
		return resp, nil
	}

	lastLogTerm := node.LastLogTerm()
	lastLogIndex := node.LastLogIndex()

	slog.Info("Last log index", "Index", lastLogIndex)
	isCandidateUptodate := (req.LastLogTerm > lastLogTerm) || (req.LastLogTerm == lastLogTerm && req.LastLogIndex >= uint64(lastLogIndex))

	if !isCandidateUptodate {
		slog.Info("candidate is not up to date, returning", "CandidateID", req.CandidateID)
		return resp, nil
	}

	resp.VoteGranted = true
	node.votedFor = req.CandidateID

	node.resetElectionTimer()
	slog.Info("vote granted", "NodeID", node.ID, "CandidateID", req.CandidateID)
	return resp, nil
}

func (node *Node) sendAppendEntries(peerID types.NodeID) {
	node.mu.RLock()

	term := node.Term
	nextIndex := node.log.nextIndex[peerID]
	prevLogIndex := nextIndex - 1
	prevLogTerm := node.TermAt(prevLogIndex)
	leaderCommit := node.log.commitIndex
	entries := make([]*types.LogEntry, 0)

	if nextIndex > 1 {
		entries = node.log.logEntries[nextIndex:]
	}

	req := &types.AppendEntriesArgs{
		Term:         term,
		LeaderID:     node.ID,
		PrevLogIndex: uint64(prevLogIndex),
		PrevLogTerm:  prevLogTerm,
		LeaderCommit: uint64(leaderCommit),
		Entries:      entries,
	}

	node.mu.RUnlock()

	resp, err := node.transport.SendAppendEntries(context.Background(), peerID, req)
	if err != nil {
		slog.Warn("unable to send append entry", "NodeID", node.ID, "PeerID", peerID)
		return
	}

	node.handleAppendEntryResponse(peerID, req, resp)
}
