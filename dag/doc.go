// Package dag executes a directed acyclic graph of work items concurrently,
// respecting their dependency edges.
//
// It is completely generic: implement the two-method [Node] interface on your
// own type and pass a slice of them. There are no external dependencies and no
// assumptions about what a "step" does.
//
//	type task struct{ name string; needs []string }
//	func (t task) ID() string         { return t.name }
//	func (t task) DependsOn() []string { return t.needs }
//
//	err := dag.Run(ctx, tasks, func(ctx context.Context, t task) error {
//		return do(ctx, t)
//	}, dag.RunOptions{MaxConcurrency: 8})
//
// The building blocks are also exported on their own:
//
//   - [TopologicalOrder] — deterministic dependency order.
//   - [Layers]           — dependency order grouped into parallelisable waves.
//   - [Validate]         — structural checks (unique IDs, no cycles, limits).
//   - [Run]              — execute a graph with a callback, bounded concurrency,
//     and dependency-aware failure handling.
//
// Partial graphs are supported: pass a subset of a larger graph (for example a
// topological slice taken to resume after a pause) and any dependency that is
// not in the subset is treated as already satisfied.
package dag
