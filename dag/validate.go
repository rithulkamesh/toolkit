package dag

import (
	"errors"
	"fmt"
)

// ValidateOptions tunes [Validate].
type ValidateOptions struct {
	// MaxNodes, when greater than zero, rejects graphs with more nodes than
	// this. Useful to bound LLM-generated or user-submitted graphs.
	MaxNodes int

	// RequireAllDeps rejects a node whose dependency ID is not present in the
	// set. When false (the default), an unknown dependency is allowed and
	// treated as already satisfied — the right choice for partial or
	// resumable graphs.
	RequireAllDeps bool
}

// Validate checks a graph's structural integrity: at least one node, non-empty
// and unique IDs, no node depending on itself, no cycles, and the
// [ValidateOptions] constraints.
func Validate[T Node](nodes []T, opts ValidateOptions) error {
	if len(nodes) == 0 {
		return errors.New("dag: graph has no nodes")
	}
	if opts.MaxNodes > 0 && len(nodes) > opts.MaxNodes {
		return fmt.Errorf("dag: %d nodes exceeds maximum of %d", len(nodes), opts.MaxNodes)
	}

	index, err := indexNodes(nodes)
	if err != nil {
		return err
	}

	for _, n := range nodes {
		id := n.ID()
		for _, dep := range n.DependsOn() {
			if dep == id {
				return fmt.Errorf("dag: node %q depends on itself", id)
			}
			if _, ok := index[dep]; !ok && opts.RequireAllDeps {
				return fmt.Errorf("dag: node %q depends on unknown node %q", id, dep)
			}
		}
	}

	if _, err := TopologicalOrder(nodes); err != nil {
		return err
	}
	return nil
}
