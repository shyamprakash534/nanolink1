package snowflake

import (
	"errors"
	"sync"
	"time"
)

const (
	epoch             = int64(1704067200000) // 2024-01-01 00:00:00 UTC
	nodeBits          = uint(10)
	sequenceBits      = uint(12)
	maxNode           = -1 ^ (-1 << nodeBits)
	maxSequence       = -1 ^ (-1 << sequenceBits)
	nodeShift         = sequenceBits
	timestampShift    = sequenceBits + nodeBits
)

// Node represents a snowflake ID generator node
type Node struct {
	mu        sync.Mutex
	epoch     int64
	node      int64
	sequence  int64
	lastStamp int64
}

// NewNode creates a new Snowflake generator node
func NewNode(nodeID int64) (*Node, error) {
	if nodeID < 0 || nodeID > maxNode {
		return nil, errors.New("node ID out of valid range (0-1023)")
	}
	return &Node{
		epoch:     epoch,
		node:      nodeID,
		sequence:  0,
		lastStamp: -1,
	}, nil
}

// Generate creates a unique 64-bit Snowflake ID
func (n *Node) Generate() uint64 {
	n.mu.Lock()
	defer n.mu.Unlock()

	now := time.Now().UnixMilli()
	if now < n.lastStamp {
		// Clock went backwards; wait or advance
		now = n.lastStamp
	}

	if now == n.lastStamp {
		n.sequence = (n.sequence + 1) & maxSequence
		if n.sequence == 0 {
			for now <= n.lastStamp {
				now = time.Now().UnixMilli()
			}
		}
	} else {
		n.sequence = 0
	}

	n.lastStamp = now
	id := uint64((now-n.epoch)<<timestampShift | (n.node << nodeShift) | n.sequence)
	return id
}
