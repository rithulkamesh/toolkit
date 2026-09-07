package dag

import (
	"fmt"
	"slices"
)

// Node is the minimal contract a graph node must satisfy. ID must be unique
// within a graph. DependsOn returns the IDs of nodes that must complete before
// this one; an empty slice means the node has no prerequisites.
type Node interface {
	ID() string
	DependsOn() []string
}

// TopologicalOrder returns nodes ordered so that every node appears after all
// of its dependencies. The order is deterministic: whenever several nodes are
// ready at once they are emitted in ascending ID order.
//
// It returns an error if a node has an empty ID, if two nodes share an ID, or
// if the graph contains a cycle.
func TopologicalOrder[T Node](nodes []T) ([]T, error) {
	index, err := indexNodes(nodes)
	if err != nil {
		return nil, err
	}

	indegree := make(map[string]int, len(nodes))
	dependents := make(map[string][]string, len(nodes))
	for id := range index {
		indegree[id] = 0
	}
	for _, n := range nodes {
		id := n.ID()
		for _, dep := range n.DependsOn() {
			if _, ok := index[dep]; !ok {
				continue // outside this set — treat as already satisfied
			}
			indegree[id]++
			dependents[dep] = append(dependents[dep], id)
		}
	}

	ready := make([]string, 0, len(nodes))
	for id, d := range indegree {
		if d == 0 {
			ready = append(ready, id)
		}
	}
	slices.Sort(ready)

	out := make([]T, 0, len(nodes))
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		out = append(out, index[id])

		var unlocked []string
		for _, dep := range dependents[id] {
			indegree[dep]--
			if indegree[dep] == 0 {
				unlocked = append(unlocked, dep)
			}
		}
		if len(unlocked) > 0 {
			ready = append(ready, unlocked...)
			slices.Sort(ready)
		}
	}

	if len(out) != len(nodes) {
		return nil, fmt.Errorf("dag: cycle detected (ordered %d of %d nodes)", len(out), len(nodes))
	}
	return out, nil
}

// Layers groups nodes into dependency "waves": every node in a layer depends
// only on nodes in earlier layers, so a caller may run all nodes in a layer
// concurrently and run the layers strictly in order.
//
// nodes may be a partial set. A dependency that is not present in nodes is
// treated as already satisfied, not as missing.
func Layers[T Node](nodes []T) ([][]T, error) {
	index, err := indexNodes(nodes)
	if err != nil {
		return nil, err
	}

	indegree := make(map[string]int, len(nodes))
	dependents := make(map[string][]string, len(nodes))
	for _, n := range nodes {
		id := n.ID()
		for _, dep := range n.DependsOn() {
			if _, ok := index[dep]; !ok {
				continue
			}
			indegree[id]++
			dependents[dep] = append(dependents[dep], id)
		}
	}

	frontier := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if indegree[n.ID()] == 0 {
			frontier = append(frontier, n.ID())
		}
	}
	slices.Sort(frontier)

	var layers [][]T
	placed := 0
	for len(frontier) > 0 {
		layer := make([]T, 0, len(frontier))
		for _, id := range frontier {
			layer = append(layer, index[id])
		}
		layers = append(layers, layer)
		placed += len(layer)

		var next []string
		for _, id := range frontier {
			for _, dep := range dependents[id] {
				indegree[dep]--
				if indegree[dep] == 0 {
					next = append(next, dep)
				}
			}
		}
		slices.Sort(next)
		frontier = next
	}

	if placed != len(nodes) {
		return nil, fmt.Errorf("dag: cycle detected (placed %d of %d nodes)", placed, len(nodes))
	}
	return layers, nil
}

func indexNodes[T Node](nodes []T) (map[string]T, error) {
	index := make(map[string]T, len(nodes))
	for _, n := range nodes {
		id := n.ID()
		if id == "" {
			return nil, fmt.Errorf("dag: node with empty ID")
		}
		if _, dup := index[id]; dup {
			return nil, fmt.Errorf("dag: duplicate node ID %q", id)
		}
		index[id] = n
	}
	return index, nil
}
