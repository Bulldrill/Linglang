package core

import "sync"

// BellPool pre-provisions and buffers Bell pairs for a QuantumChannel, so
// that Teleport (#14) can consume an already-entangled pair instead of
// paying pair-generation latency on the protocol's critical path — the
// "pre-provisioning protocol" of issue #17.
//
// BellPool itself implements QuantumChannel (it just wraps one), so
// core.Teleport(psi, pool) works exactly like core.Teleport(psi, channel):
// ShareBellPair serves from the buffer when it can, falling back to the
// wrapped channel's ShareBellPair on a pool miss, which is still correct —
// just not pre-provisioned, like any cache miss.
type BellPool struct {
	channel QuantumChannel
	target  int

	mu     sync.Mutex
	buffer []*BellPair
}

// NewBellPool wraps channel with a pool that tries to keep target pairs
// pre-provisioned.
func NewBellPool(channel QuantumChannel, target int) *BellPool {
	return &BellPool{channel: channel, target: target}
}

func (p *BellPool) NodeA() string { return p.channel.NodeA() }
func (p *BellPool) NodeB() string { return p.channel.NodeB() }

func (p *BellPool) SendClassical(from string, bits []int) error {
	return p.channel.SendClassical(from, bits)
}

func (p *BellPool) RecvClassical(to string) ([]int, error) {
	return p.channel.RecvClassical(to)
}

// ShareBellPair serves a pre-provisioned pair from the buffer if one is
// available, otherwise generates one on demand via the wrapped channel.
func (p *BellPool) ShareBellPair() (*BellPair, error) {
	p.mu.Lock()
	if len(p.buffer) > 0 {
		pair := p.buffer[0]
		p.buffer = p.buffer[1:]
		p.mu.Unlock()
		return pair, nil
	}
	p.mu.Unlock()
	return p.channel.ShareBellPair()
}

// Refill tops the buffer up to its target size, generating pairs eagerly
// (e.g. called periodically from a background goroutine, or ahead of a
// batch of expected teleportations) so later ShareBellPair calls rarely
// have to generate on the critical path.
func (p *BellPool) Refill() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for len(p.buffer) < p.target {
		pair, err := p.channel.ShareBellPair()
		if err != nil {
			return err
		}
		p.buffer = append(p.buffer, pair)
	}
	return nil
}

// Available reports how many pre-provisioned pairs are ready right now.
func (p *BellPool) Available() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.buffer)
}
