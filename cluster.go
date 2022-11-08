package raft

import (
	"log"
	"math/rand"
	"testing"
	"time"
)

// Cluster is a collection of nodes.
type Cluster struct {
	cluster   []*Server
	connected []bool
	n         int
	t         *testing.T
}

func NewCluster(t *testing.T, n int) *Cluster {
	ns := make([]*Server, n)
	connected := make([]bool, n)
	ready := make(chan interface{})

	// Create all Servers in this cluster, assign ids and peer ids.
	for i := 0; i < n; i++ {
		peerIds := make([]int, 0)
		for p := 0; p < n; p++ {
			if p != i {
				peerIds = append(peerIds, p)
			}
		}
		ns[i] = NewServer(i, peerIds, ready)
		ns[i].Serve()
	}

	// Connect all peers to each other.
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if i != j {
				ns[i].ConnectToPeer(j, ns[j].GetListenAddr())
			}
		}
		connected[i] = true
	}
	close(ready)

	return &Cluster{
		cluster:   ns,
		connected: connected,
		n:         n,
		t:         t,
	}
}

func (c *Cluster) DisconnectPeer(id int) {
	tlog("Disconnect %d", id)
	c.cluster[id].DisconnectAll()
	for j := 0; j < c.n; j++ {
		if j != id {
			c.cluster[j].DisconnectPeer(id)
		}
	}
	c.connected[id] = false
}

func (c *Cluster) ReconnectPeer(id int) {
	tlog("Reconnect %d", id)
	for j := 0; j < c.n; j++ {
		if j != id {
			if err := c.cluster[id].ConnectToPeer(j, c.cluster[j].GetListenAddr()); err != nil {
				c.t.Fatal(err)
			}
			if err := c.cluster[j].ConnectToPeer(id, c.cluster[id].GetListenAddr()); err != nil {
				c.t.Fatal(err)
			}
		}
	}
	c.connected[id] = true
}

func (c *Cluster) Shutdown() {
	for i := 0; i < c.n; i++ {
		c.cluster[i].DisconnectAll()
		c.connected[i] = false
	}
	for i := 0; i < c.n; i++ {
		c.cluster[i].Shutdown()
	}
}

func (c *Cluster) CheckOneLeader() (int, int) {
	for r := 0; r < 5; r++ {
		leaderId := -1
		leaderTerm := -1
		for i := 0; i < c.n; i++ {
			if c.connected[i] {
				_, term, isLeader := c.cluster[i].cm.Report()
				if isLeader {
					if leaderId < 0 {
						leaderId = i
						leaderTerm = term
					} else {
						c.t.Fatalf("Both %d and %d think they're leaders", leaderId, i)
					}
				}
			}
		}
		if leaderId >= 0 {
			return leaderId, leaderTerm
		}
		time.Sleep(150 * time.Millisecond)
	}

	c.t.Fatalf("Leader not found")
	return -1, -1
}

func (c *Cluster) CheckNoLeader() {
	for i := 0; i < c.n; i++ {
		if c.connected[i] {
			_, _, isLeader := c.cluster[i].cm.Report()
			if isLeader {
				c.t.Fatalf("Server %d Leader; want none", i)
			}
		}
	}
}

func init() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	rand.Seed(time.Now().UnixNano())
}

func tlog(format string, a ...interface{}) {
	format = "[TEST] " + format
	log.Printf(format, a...)
}

func sleepMs(n int) {
	time.Sleep(time.Duration(n) * time.Millisecond)
}