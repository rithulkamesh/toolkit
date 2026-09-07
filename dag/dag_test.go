package dag

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type node struct {
	id   string
	deps []string
}

func (n node) ID() string          { return n.id }
func (n node) DependsOn() []string { return n.deps }

func ids[T Node](ns []T) []string {
	out := make([]string, len(ns))
	for i, n := range ns {
		out[i] = n.ID()
	}
	return out
}

func TestTopologicalOrder(t *testing.T) {
	nodes := []node{
		{id: "d", deps: []string{"b", "c"}},
		{id: "b", deps: []string{"a"}},
		{id: "c", deps: []string{"a"}},
		{id: "a"},
	}
	got, err := TopologicalOrder(nodes)
	if err != nil {
		t.Fatal(err)
	}
	// Deterministic: a, then b & c in ID order, then d.
	want := []string{"a", "b", "c", "d"}
	if fmt.Sprint(ids(got)) != fmt.Sprint(want) {
		t.Fatalf("got %v want %v", ids(got), want)
	}
}

func TestTopologicalOrderCycle(t *testing.T) {
	nodes := []node{
		{id: "a", deps: []string{"b"}},
		{id: "b", deps: []string{"a"}},
	}
	if _, err := TopologicalOrder(nodes); err == nil {
		t.Fatal("expected cycle error")
	}
}

func TestIndexErrors(t *testing.T) {
	if _, err := TopologicalOrder([]node{{id: ""}}); err == nil {
		t.Fatal("expected empty-ID error")
	}
	if _, err := TopologicalOrder([]node{{id: "x"}, {id: "x"}}); err == nil {
		t.Fatal("expected duplicate-ID error")
	}
}

func TestLayers(t *testing.T) {
	nodes := []node{
		{id: "a"},
		{id: "b", deps: []string{"a"}},
		{id: "c", deps: []string{"a"}},
		{id: "d", deps: []string{"b", "c"}},
	}
	layers, err := Layers(nodes)
	if err != nil {
		t.Fatal(err)
	}
	if len(layers) != 3 {
		t.Fatalf("want 3 layers, got %d: %v", len(layers), layers)
	}
	if fmt.Sprint(ids(layers[0])) != "[a]" || fmt.Sprint(ids(layers[1])) != "[b c]" || fmt.Sprint(ids(layers[2])) != "[d]" {
		t.Fatalf("unexpected layering: %v %v %v", ids(layers[0]), ids(layers[1]), ids(layers[2]))
	}
}

func TestLayersPartialSet(t *testing.T) {
	// "b" depends on "a", but "a" is not in the slice — treated as satisfied.
	nodes := []node{
		{id: "b", deps: []string{"a"}},
		{id: "c", deps: []string{"b"}},
	}
	layers, err := Layers(nodes)
	if err != nil {
		t.Fatal(err)
	}
	if len(layers) != 2 || fmt.Sprint(ids(layers[0])) != "[b]" {
		t.Fatalf("unexpected: %v", layers)
	}
}

func TestValidate(t *testing.T) {
	ok := []node{{id: "a"}, {id: "b", deps: []string{"a"}}}
	if err := Validate(ok, ValidateOptions{}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if err := Validate(ok, ValidateOptions{MaxNodes: 1}); err == nil {
		t.Fatal("expected MaxNodes error")
	}
	if err := Validate([]node{{id: "a", deps: []string{"a"}}}, ValidateOptions{}); err == nil {
		t.Fatal("expected self-dependency error")
	}
	if err := Validate([]node{{id: "a", deps: []string{"ghost"}}}, ValidateOptions{RequireAllDeps: true}); err == nil {
		t.Fatal("expected unknown-dependency error")
	}
	if err := Validate([]node{}, ValidateOptions{}); err == nil {
		t.Fatal("expected empty-graph error")
	}
}

func TestRunOrderAndConcurrency(t *testing.T) {
	nodes := []node{
		{id: "root"},
		{id: "a", deps: []string{"root"}},
		{id: "b", deps: []string{"root"}},
		{id: "c", deps: []string{"root"}},
		{id: "join", deps: []string{"a", "b", "c"}},
	}

	var mu sync.Mutex
	started := map[string]time.Time{}
	done := map[string]time.Time{}
	var inFlight, maxInFlight int32

	err := Run(context.Background(), nodes, func(_ context.Context, n node) error {
		cur := atomic.AddInt32(&inFlight, 1)
		for {
			m := atomic.LoadInt32(&maxInFlight)
			if cur <= m || atomic.CompareAndSwapInt32(&maxInFlight, m, cur) {
				break
			}
		}
		mu.Lock()
		started[n.id] = time.Now()
		mu.Unlock()

		time.Sleep(30 * time.Millisecond)

		mu.Lock()
		done[n.id] = time.Now()
		mu.Unlock()
		atomic.AddInt32(&inFlight, -1)
		return nil
	}, RunOptions{MaxConcurrency: 4})
	if err != nil {
		t.Fatal(err)
	}

	// join must start after a, b, c all finished.
	for _, dep := range []string{"a", "b", "c"} {
		if !started["join"].After(done[dep]) {
			t.Fatalf("join started before %s finished", dep)
		}
	}
	// a, b, c should have overlapped.
	if maxInFlight < 2 {
		t.Fatalf("expected concurrent execution, max in flight was %d", maxInFlight)
	}
}

func TestRunFailurePropagation(t *testing.T) {
	nodes := []node{
		{id: "a"},
		{id: "b", deps: []string{"a"}},
		{id: "c", deps: []string{"b"}},
		{id: "d"}, // independent — still runs
	}
	var ran sync.Map
	err := Run(context.Background(), nodes, func(_ context.Context, n node) error {
		ran.Store(n.id, true)
		if n.id == "a" {
			return errors.New("boom")
		}
		return nil
	}, RunOptions{ContinueOnError: true})

	if err == nil {
		t.Fatal("expected error")
	}
	if _, ok := ran.Load("b"); ok {
		t.Fatal("b should have been skipped after a failed")
	}
	if _, ok := ran.Load("c"); ok {
		t.Fatal("c should have been skipped (cascade)")
	}
	if _, ok := ran.Load("d"); !ok {
		t.Fatal("independent node d should still have run")
	}
}

func TestRunMaxConcurrencyOne(t *testing.T) {
	nodes := []node{{id: "a"}, {id: "b"}, {id: "c"}}
	var inFlight, maxInFlight int32
	err := Run(context.Background(), nodes, func(_ context.Context, _ node) error {
		cur := atomic.AddInt32(&inFlight, 1)
		if cur > atomic.LoadInt32(&maxInFlight) {
			atomic.StoreInt32(&maxInFlight, cur)
		}
		time.Sleep(10 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		return nil
	}, RunOptions{MaxConcurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	if maxInFlight != 1 {
		t.Fatalf("MaxConcurrency:1 violated, max in flight was %d", maxInFlight)
	}
}
