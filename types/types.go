package types

type NodeID int64

type LogEntry struct {
	Term    uint64
	Index   uint64
	Command []byte
}

type AppendEntriesArgs struct {
	Term         uint64
	LeaderID     NodeID
	PrevLogIndex uint64
	PrevLogTerm  uint64
	LeaderCommit uint64 //Leaders commit index
	Entries      []*LogEntry
}

type AppendEntriesResult struct {
	Term    uint64
	Success bool
}

type RequestVoteArgs struct {
	Term         uint64
	CandidateID  NodeID
	LastLogIndex uint64
	LastLogTerm  uint64
}

type RequestVoteResult struct {
	Term        uint64
	VoteGranted bool
}
