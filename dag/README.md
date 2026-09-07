# dag

Run a directed acyclic graph of work concurrently, in dependency order. Generic,
zero-dependency, ~250 lines.

## Why

Any time you have "steps that depend on other steps" — a build pipeline, an
ETL job, an AI workflow (retrieve context → call model → take action → notify),
a fan-out/fan-in of API calls — you want three things: a stable order, maximum
safe parallelism, and sane behaviour when one step fails. This package is those
three things and nothing else.

## The contract

Implement two methods:

```go
type Node interface {
	ID() string          // unique within the graph
	DependsOn() []string  // IDs that must finish first
}
```

## Use

```go
type task struct {
	name string
	needs []string
}
func (t task) ID() string         { return t.name }
func (t task) DependsOn() []string { return t.needs }

tasks := []task{
	{name: "fetch"},
	{name: "parse",  needs: []string{"fetch"}},
	{name: "enrich", needs: []string{"fetch"}},
	{name: "write",  needs: []string{"parse", "enrich"}},
}

err := dag.Run(ctx, tasks, func(ctx context.Context, t task) error {
	return process(ctx, t)
}, dag.RunOptions{MaxConcurrency: 8})
```

`parse` and `enrich` run in parallel; `write` waits for both.

## Behaviour

- **Deterministic order.** When several nodes are ready at once they run in
  ascending ID order (and `TopologicalOrder` emits them that way).
- **Failure cascades.** If a node's `StepFunc` returns an error, every node that
  transitively depends on it is skipped. The returned error is
  `errors.Join` of each failure, wrapped with its node ID.
- **`ContinueOnError: false`** (default) stops after the current layer once any
  node fails. `true` keeps running nodes whose dependencies all succeeded.
- **Context cancellation** stops scheduling new nodes; in-flight nodes get the
  cancelled `ctx` and are expected to return promptly.
- **Partial graphs.** Pass a subset of a larger graph; a dependency that is not
  in the subset counts as already satisfied. This is how you resume a graph
  after a pause (persist progress, re-submit the remaining slice).

## Building blocks (used independently)

| Function | Returns |
|---|---|
| `TopologicalOrder(nodes)` | `[]T` in dependency order, or a cycle error |
| `Layers(nodes)` | `[][]T` — each inner slice is safe to run in parallel |
| `Validate(nodes, opts)` | structural error: empty/dup IDs, self-dep, cycle, `MaxNodes` |
| `Run(ctx, nodes, fn, opts)` | executes with bounded concurrency + failure handling |

## Not included

Persistence, retries, backoff, distributed execution. Wrap `StepFunc` for
retries; persist your own progress and re-submit the remaining nodes to resume.
