# Go-Raft
Implementation of Raft Consensus Algorithm in Go language

## So what is Raft?
Raft is a consensus algorithm equivalent to Paxos. It is designed to be easy to understand. It's a leader-based algorithm, which means that all writes from the clients are forwarded to the leader. The leader then replicates the log entries to the followers. Leader sends RPC's (Heartbeats and Append Entries) to the followers in order to keep them in sync. If the leader fails, one of the followers will become the leader. This is done by electing a new leader through the votes of the followers. If a leader gets a majority of votes, it becomes the leader. 