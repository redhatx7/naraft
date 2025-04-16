package raft

import "github.com/redhatx7/naraft/types"

type LogManager struct {
	logEntries  []*types.LogEntry
	commitIndex int

	matchIndex map[types.NodeID]int
	nextIndex  map[types.NodeID]int
}

func NewLogManager(peers []types.NodeID) *LogManager {
	logManager := &LogManager{
		logEntries:  make([]*types.LogEntry, 0),
		commitIndex: -1,
		matchIndex:  make(map[types.NodeID]int),
		nextIndex:   make(map[types.NodeID]int),
	}
	for _, peerID := range peers {
		logManager.matchIndex[peerID] = -1
		logManager.nextIndex[peerID] = 0
	}
	return logManager
}

func (node *Node) LastLogIndex() int {
	return len(node.log.logEntries) - 1
}

func (node *Node) LastLogTerm() uint64 {
	if len(node.log.logEntries) == 0 {
		return 0
	}
	return node.log.logEntries[len(node.log.logEntries)-1].Term
}

func (node *Node) TermAt(idx int) uint64 {
	if idx < 0 || idx >= len(node.log.logEntries) {
		return 0
	}
	return node.log.logEntries[idx].Term
}

func (node *Node) TruncateLog(req *types.AppendEntriesArgs) {

	insertIdx := int(req.PrevLogIndex)

	for i, entry := range req.Entries {
		idx := insertIdx + i + 1

		if idx < len(node.log.logEntries) {
			if node.log.logEntries[idx].Term != entry.Term {
				// Conflict, delete
				node.log.logEntries = node.log.logEntries[:idx-1]
				break
			} // Else => term match at that index, continue iterating
		} else {
			// No entry at current index

			node.log.logEntries = append(node.log.logEntries, req.Entries[i:]...)
			break
		}
	}

	if req.LeaderCommit > uint64(node.log.commitIndex) {
		lastEntryIndex := req.PrevLogIndex + uint64(len(node.log.logEntries))
		node.log.commitIndex = int(min(req.LeaderCommit, lastEntryIndex))
	}

}

func (node *Node) LogLen() int {
	return len(node.log.logEntries)
}
