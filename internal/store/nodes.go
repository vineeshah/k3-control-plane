package store

import (
	"sort"
	"time"

	"k8/internal/api"
)

func (s *MemoryStore) UpsertNode(node api.Node) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodes[node.ID] = node.Clone()
}

func (s *MemoryStore) TouchNode(id string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	node, ok := s.nodes[id]
	if !ok {
		return false
	}
	node.LastHeartbeat = now
	s.nodes[id] = node
	return true
}

func (s *MemoryStore) ListNodes() []api.Node {
	s.mu.RLock()
	defer s.mu.RUnlock()

	nodes := make([]api.Node, 0, len(s.nodes))
	for _, node := range s.nodes {
		nodes = append(nodes, node.Clone())
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	return nodes
}
