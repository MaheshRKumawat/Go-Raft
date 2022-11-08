// All structs for Raft Consensus are here.
// which contain Consensus Module(CM), Server, CM's state, and CM's log etc

package raft

import (
	"sync"
	"time"
)

type CMState int

const DebugCM int = 1

const (
	Follower CMState = iota
	Candidate
	Leader
	Dead
)

// ConsensusModule is the main struct for the Raft consensus module.
type ConsensusModule struct {
	id      int        // this peer's id
	mu      sync.Mutex // Lock to protect shared access to this peer's state
	peerIds []int      // ids of all peers (including self)
	server  *Server    // RPC end point to receive/send RPCs from/to peers

	// persistent state on all servers
	currentTerm int        // latest term server has seen (initialized to 0 on first boot, increases monotonically)
	votedFor    int        // candidateId that received vote in current term (or null if none)
	log         []LogEntry // log entries; each entry contains command for state machine, and term when entry was received by leader (first index is 1)

	// volatile state on all servers
	state              CMState   // state of this CM (Follower, Candidate, Leader, Dead, Shutdown)
	electionResetEvent time.Time // used to reset election timer
}

// LogEntry is a single entry in the log.
type LogEntry struct {
	Command interface{}
	Term    int
}

type RPCProxy struct {
	cm *ConsensusModule
}

// RequestVoteArgs is the arguments for the RequestVote RPC.
type RequestVoteArgs struct {
	Term         int
	CandidateId  int
	LastLogIndex int
	LastLogTerm  int
}

// RequestVoteReply is the reply for the RequestVote RPC.
type RequestVoteReply struct {
	Term        int
	VoteGranted bool
}

// AppendEntriesArgs is the arguments for the AppendEntries RPC.
type AppendEntriesArgs struct {
	Term         int
	LeaderId     int
	PrevLogIndex int
	PrevLogTerm  int
	LeaderCommit int
	Entries      []LogEntry
}

// AppendEntriesReply is the reply for the AppendEntries RPC.
type AppendEntriesReply struct {
	Term    int
	Success bool
}
