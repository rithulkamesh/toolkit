package dag

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// StepFunc executes a single node. Returning a non-nil error marks that node
// as failed.
type StepFunc[T Node] func(ctx context.Context, node T) error

// RunOptions tunes [Run].
type RunOptions struct {
	// MaxConcurrency bounds how many nodes run at once within a layer. Zero or
	// negative means no bound (every node in a layer runs in parallel).
	MaxConcurrency int

	// ContinueOnError keeps running nodes whose dependencies all succeeded
	// even after another node has failed. By default Run stops after the
	// current layer once any node fails. Either way, every node that fails or
	// is skipped is reported in the joined error.
	ContinueOnError bool
}

// Run executes nodes in dependency order. Independent nodes within a layer run
// concurrently, bounded by RunOptions.MaxConcurrency.
//
// A node is skipped when any of its dependencies failed or was itself skipped;
// skips cascade. Run returns the joined errors of every failed node (each
// wrapped with its node ID), or nil when every node succeeded. A cancelled
// ctx stops scheduling new nodes and is included in the returned error.
func Run[T Node](ctx context.Context, nodes []T, fn StepFunc[T], opts RunOptions) error {
	layers, err := Layers(nodes)
	if err != nil {
		return err
	}

	var (
		mu     sync.Mutex
		failed = make(map[string]struct{})
		errs   []error
	)

	markFailed := func(id string, err error) {
		mu.Lock()
		failed[id] = struct{}{}
		if err != nil {
			errs = append(errs, err)
		}
		mu.Unlock()
	}
	depFailed := func(n T) bool {
		mu.Lock()
		defer mu.Unlock()
		for _, dep := range n.DependsOn() {
			if _, bad := failed[dep]; bad {
				return true
			}
		}
		return false
	}

layers:
	for _, layer := range layers {
		if ctx.Err() != nil {
			break
		}

		var sem chan struct{}
		if opts.MaxConcurrency > 0 {
			sem = make(chan struct{}, opts.MaxConcurrency)
		}
		var wg sync.WaitGroup

		for _, node := range layer {
			if depFailed(node) {
				markFailed(node.ID(), nil) // skip; cascade to dependents
				continue
			}
			if ctx.Err() != nil {
				break
			}

			wg.Add(1)
			if sem != nil {
				sem <- struct{}{}
			}
			go func(n T) {
				defer wg.Done()
				if sem != nil {
					defer func() { <-sem }()
				}
				if err := fn(ctx, n); err != nil {
					markFailed(n.ID(), fmt.Errorf("node %q: %w", n.ID(), err))
				}
			}(node)
		}

		wg.Wait()

		mu.Lock()
		stop := len(errs) > 0 && !opts.ContinueOnError
		mu.Unlock()
		if stop {
			break layers
		}
	}

	if ctx.Err() != nil {
		errs = append(errs, ctx.Err())
	}
	return errors.Join(errs...)
}
