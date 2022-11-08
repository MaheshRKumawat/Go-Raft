package raft

import (
	"testing"
	"time"

	"github.com/fortytw2/leaktest"
)

const n int = 5

// basic test
func Test_1(t *testing.T) {
	cluster := NewCluster(t, n)
	defer cluster.Shutdown()

	cluster.CheckOneLeader()
}

// When leader is disconnected, a new leader should be elected.
func Test_2(t *testing.T) {
	cluster := NewCluster(t, n)
	defer cluster.Shutdown()

	origLeaderId, origTerm := cluster.CheckOneLeader()

	cluster.DisconnectPeer(origLeaderId)
	sleepMs(350)

	newLeaderId, newTerm := cluster.CheckOneLeader()
	if newLeaderId == origLeaderId {
		t.Errorf("want new leader to be different from orig leader")
	}
	if newTerm <= origTerm {
		t.Errorf("want newTerm <= origTerm, got %d and %d", newTerm, origTerm)
	}
}

// When a leader and a follower are disconnected, a new leader should be elected.
func Test_3(t *testing.T) {
	cluster := NewCluster(t, n)
	defer cluster.Shutdown()

	origLeaderId, _ := cluster.CheckOneLeader()

	cluster.DisconnectPeer(origLeaderId)
	otherId := (origLeaderId + 1) % n
	cluster.DisconnectPeer(otherId)

	// No quorum.
	sleepMs(450)
	cluster.CheckNoLeader()

	// Reconnect one other server; now we'll have quorum.
	cluster.ReconnectPeer(otherId)
	cluster.CheckOneLeader()
}

// When all nodes are disconnected.
func Test_4(t *testing.T) {
	cluster := NewCluster(t, n)
	defer cluster.Shutdown()

	sleepMs(100)
	//	Disconnect all servers from the start. There will be no leader.
	for i := 0; i < n; i++ {
		cluster.DisconnectPeer(i)
	}
	sleepMs(450)
	cluster.CheckNoLeader()

	// Reconnect all servers. A leader will be found.
	for i := 0; i < n; i++ {
		cluster.ReconnectPeer(i)
	}
	cluster.CheckOneLeader()
}

// When a leader is disconnected and reconnected
func Test_5(t *testing.T) {
	cluster := NewCluster(t, n)
	defer cluster.Shutdown()
	origLeaderId, _ := cluster.CheckOneLeader()

	cluster.DisconnectPeer(origLeaderId)

	sleepMs(350)
	newLeaderId, newTerm := cluster.CheckOneLeader()

	cluster.ReconnectPeer(origLeaderId)
	sleepMs(150)

	againLeaderId, againTerm := cluster.CheckOneLeader()

	if newLeaderId != againLeaderId {
		t.Errorf("again leader id got %d; want %d", againLeaderId, newLeaderId)
	}
	if againTerm != newTerm {
		t.Errorf("again term got %d; want %d", againTerm, newTerm)
	}
}

// When a follower is disconnected and reconnected
func Test_6(t *testing.T) {
	defer leaktest.CheckTimeout(t, 100*time.Millisecond)()

	cluster := NewCluster(t, n)
	defer cluster.Shutdown()

	origLeaderId, origTerm := cluster.CheckOneLeader()

	otherId := (origLeaderId + 1) % n
	cluster.DisconnectPeer(otherId)
	time.Sleep(650 * time.Millisecond)
	cluster.ReconnectPeer(otherId)
	sleepMs(150)

	_, newTerm := cluster.CheckOneLeader()
	if newTerm <= origTerm {
		t.Errorf("newTerm=%d, origTerm=%d", newTerm, origTerm)
	}
}

// During election disconnect loop
func Test_7(t *testing.T) {
	defer leaktest.CheckTimeout(t, 100*time.Millisecond)()

	cluster := NewCluster(t, n)
	defer cluster.Shutdown()

	for cycle := 0; cycle < 5; cycle++ {
		leaderId, _ := cluster.CheckOneLeader()

		cluster.DisconnectPeer(leaderId)
		otherId := (leaderId + 1) % n
		cluster.DisconnectPeer(otherId)
		sleepMs(310)
		cluster.CheckNoLeader()

		// Reconnect both.
		cluster.ReconnectPeer(otherId)
		cluster.ReconnectPeer(leaderId)

		// Give it time to settle
		sleepMs(150)
	}
}
