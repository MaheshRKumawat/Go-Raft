package raft

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"
)

func NewConsensusModule(id int, peerIds []int, server *Server, ready <-chan interface{}) *ConsensusModule {
	cm := new(ConsensusModule)
	cm.id = id
	cm.peerIds = peerIds
	cm.server = server
	cm.state = Follower
	cm.votedFor = -1

	// Election timer is reset when a message is received from the leader or on conversion to candidate.
	// It is also reset on conversion to follower.
	go func() {
		<-ready
		cm.mu.Lock()
		cm.electionResetEvent = time.Now()
		cm.mu.Unlock()
		cm.runElectionTimer()
	}()

	return cm
}

func (cm *ConsensusModule) startElection() {
	cm.state = Candidate
	cm.currentTerm += 1
	savedCurrentTerm := cm.currentTerm
	cm.electionResetEvent = time.Now()
	cm.votedFor = cm.id
	cm.dlog("Becomes Candidate Current_Term=%d; Log=%v", savedCurrentTerm, cm.log)

	votesReceived := 1

	// Send RequestVote RPCs to all other servers concurrently.
	for _, peerId := range cm.peerIds {
		go func(peerId int) {
			args := RequestVoteArgs{
				Term:        savedCurrentTerm,
				CandidateId: cm.id,
			}
			var reply RequestVoteReply

			cm.dlog("Sending RequestVote to %d: %+v", peerId, args)
			ok := cm.server.Call(peerId, "ConsensusModule.RequestVote", args, &reply)
			if ok == nil {
				cm.mu.Lock()
				defer cm.mu.Unlock()
				cm.dlog("Received RequestVoteReply %+v", reply)

				if cm.state != Candidate {
					cm.dlog("while waiting for reply, state = %v", cm.state)
					return
				}

				if reply.Term > savedCurrentTerm {
					cm.dlog("Term out of date in RequestVoteReply")
					cm.becomeFollower(reply.Term)
					return
				} else if reply.Term == savedCurrentTerm {
					if reply.VoteGranted {
						votesReceived += 1
						if votesReceived > ((len(cm.peerIds)+1)/2) {
							// Won the election!
							cm.dlog("Wins Election with %d votes", votesReceived)
							cm.becomeLeader()
							return
						}
					}
				}
			}
		}(peerId)
	}

	// If election timeout elapses without majority votes, start a new election
	go cm.runElectionTimer()
}

// RequestVote is sent by a candidate to each peer to request their vote.
func (cm *ConsensusModule) RequestVote(args RequestVoteArgs, reply *RequestVoteReply) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if cm.state == Dead {
		return nil
	}
	cm.dlog("RequestVote=%+v, Current_Term=%d, votedFor=%d", args, cm.currentTerm, cm.votedFor)

	if args.Term > cm.currentTerm {
		cm.dlog("Term out of date in RequestVote")
		cm.becomeFollower(args.Term)
	}

	if cm.currentTerm == args.Term &&
		(cm.votedFor == -1 || cm.votedFor == args.CandidateId) {
		reply.VoteGranted = true
		cm.votedFor = args.CandidateId
		cm.electionResetEvent = time.Now()
	} else {
		reply.VoteGranted = false
	}
	reply.Term = cm.currentTerm
	cm.dlog("RequestVote Reply= %+v", reply)
	return nil
}

// go routine that runs in background in every node except the leader
func (cm *ConsensusModule) runElectionTimer() {

	// electionTimeout should be a random duration between 150 and 300 milliseconds.
	// It should be more than the heartbeat interval and the RPC for AppendEntries.
	var additionalTime int
	if len(os.Getenv("DISABLE_ELECTION_TIMER")) > 0 && rand.Intn(3) == 0 {
		additionalTime = 0
	} else {
		additionalTime = rand.Intn(150)
	}

	timeoutDuration := time.Duration(150+additionalTime) * time.Millisecond
	cm.mu.Lock()
	termStarted := cm.currentTerm
	cm.mu.Unlock()
	cm.dlog("Election Timer Started=%v, Term=%d", timeoutDuration, termStarted)

	// This loops until either:
	// - we discover the election timer is no longer needed, or
	// - the election timer expires and this CM becomes a candidate
	// In a follower, this typically keeps running in the background for the
	// duration of the CM's lifetime.
	electionTimer := time.NewTicker(10 * time.Millisecond)
	defer electionTimer.Stop()
	for {
		<-electionTimer.C

		cm.mu.Lock()
		if cm.state != Candidate && cm.state != Follower {
			cm.dlog("Election timer stopped because state is %s", cm.state)
			cm.mu.Unlock()
			return
		}

		if termStarted != cm.currentTerm {
			cm.dlog("Election timer stopped because term changed from %v to %v", termStarted, cm.currentTerm)
			cm.mu.Unlock()
			return
		}

		// Start an election if we haven't heard from a leader or haven't voted for
		// someone for the duration of the timeout.
		if elapsed := time.Since(cm.electionResetEvent); elapsed >= timeoutDuration {
			cm.startElection()
			cm.mu.Unlock()
			return
		}
		cm.mu.Unlock()
	}
}

func (cm *ConsensusModule) dlog(format string, args ...interface{}) {
	if DebugCM > 0 {
		format = fmt.Sprintf("[%d] ", cm.id) + format
		log.Printf(format, args...)
	}
}

// Append Entries only sent by a Leader in a praticular term to every follower
func (cm *ConsensusModule) AppendEntries(args AppendEntriesArgs, reply *AppendEntriesReply) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if cm.state == Dead {
		return nil
	}
	cm.dlog("AppendEntries= %+v", args)

	if args.Term > cm.currentTerm {
		cm.dlog("Term out of date in AppendEntries")
		cm.becomeFollower(args.Term)
	}

	reply.Success = false
	if args.Term == cm.currentTerm {
		if cm.state != Follower {
			cm.becomeFollower(args.Term)
		}
		cm.electionResetEvent = time.Now()
		reply.Success = true
	}

	reply.Term = cm.currentTerm
	cm.dlog("AppendEntries reply= %+v", *reply)
	return nil
}

// Only leader sends heartbeats to followers.
// Heartbeats are sent periodically, and also when a leader first comes to power,
// Each heartbeats are empty append entries except containing current term and leader id.
func (cm *ConsensusModule) leaderSendHeartbeats() {
	cm.mu.Lock()
	if cm.state != Leader {
		cm.mu.Unlock()
		return
	}
	savedCurrentTerm := cm.currentTerm
	cm.mu.Unlock()

	for _, peerId := range cm.peerIds {
		args := AppendEntriesArgs{
			Term:     savedCurrentTerm,
			LeaderId: cm.id,
		}
		go func(peerId int) {
			cm.dlog("Sending AppendEntries to Peer %v: ni=%d, args=%+v", peerId, 0, args)
			var reply AppendEntriesReply
			ok := cm.server.Call(peerId, "ConsensusModule.AppendEntries", args, &reply)
			if ok == nil {
				cm.mu.Lock()
				defer cm.mu.Unlock()
				if reply.Term > savedCurrentTerm {
					cm.dlog("term out of date in heartbeat reply")
					cm.becomeFollower(reply.Term)
					return
				}
			}
		}(peerId)
	}
}

func (cm *ConsensusModule) becomeLeader() {
	cm.state = Leader
	cm.dlog("Becomes Leader with Term=%d, Log=%v", cm.currentTerm, cm.log)

	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()

		// Send periodic heartbeats, as long as still leader.
		for {
			cm.leaderSendHeartbeats()
			<-ticker.C

			cm.mu.Lock()
			if cm.state != Leader {
				cm.mu.Unlock()
				return
			}
			cm.mu.Unlock()
		}
	}()
}

func (cm *ConsensusModule) becomeFollower(term int) {
	cm.dlog("Becomes Follower with Term=%d; Log=%v", term, cm.log)
	cm.state = Follower
	cm.currentTerm = term
	cm.votedFor = -1
	cm.electionResetEvent = time.Now()
	go cm.runElectionTimer()
}



// reports the state of this CM.
func (cm *ConsensusModule) Report() (id int, term int, isLeader bool) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.id, cm.currentTerm, cm.state == Leader
}

// When a node is unreachable or crashed it is considered dead
func (cm *ConsensusModule) Stop() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.state = Dead
	cm.dlog("Becomes Dead")
}