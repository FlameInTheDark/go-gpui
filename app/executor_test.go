package app

import (
	"container/heap"
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Background executor (upstream executor.rs: spawn, priorities, Task)
// ---------------------------------------------------------------------------

// gatePool saturates a fresh background executor's four workers with tasks
// blocked on the returned release channel, and reports when every worker is
// actually occupied. Batch tasks spawned afterwards are guaranteed to sit
// in the priority queue instead of running.
func gatePool(t *testing.T, be *BackgroundExecutor) (started <-chan struct{}, release chan struct{}) {
	t.Helper()
	const workers = 4
	release = make(chan struct{})
	startedCh := make(chan struct{}, workers)
	for i := 0; i < workers; i++ {
		be.SpawnWithPriority(PriorityHigh, func(context.Context) {
			startedCh <- struct{}{}
			<-release
		})
	}
	started = startedCh
	// Wait until all four workers are parked in a gate.
	for i := 0; i < workers; i++ {
		select {
		case <-startedCh:
		case <-time.After(2 * time.Second):
			t.Fatal("gate tasks did not start")
		}
	}
	return started, release
}

// waitDone joins a task with a timeout so a broken executor fails the test
// instead of hanging it.
func waitDone(t *testing.T, task *BackgroundTask) {
	t.Helper()
	select {
	case <-task.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("task did not finish in time")
	}
}

// TestBackgroundExecutorSpawnRuns verifies the legacy Spawn API still
// executes closures on the pool (call-site compatibility).
func TestBackgroundExecutorSpawnRuns(t *testing.T) {
	be := NewBackgroundExecutor()
	var mu sync.Mutex
	ran := false
	task := be.Spawn(func() {
		mu.Lock()
		ran = true
		mu.Unlock()
	})
	waitDone(t, task)
	mu.Lock()
	defer mu.Unlock()
	if !ran {
		t.Fatal("spawned task did not run")
	}
}

// TestTaskHeapOrdering verifies the queue ordering deterministically at the
// data-structure level: higher priority first, FIFO (enqueue seq) within a
// level (upstream PriorityQueue semantics).
func TestTaskHeapOrdering(t *testing.T) {
	h := &taskHeap{}
	push := func(priority Priority, seq uint64, name string) {
		heap.Push(h, &taskEntry{priority: priority, seq: seq, run: func(context.Context) {}, task: &BackgroundTask{}, name: name})
	}
	// Enqueue interleaved priorities and out-of-order seqs within a level.
	push(PriorityIdle, 5, "idle")
	push(PriorityHigh, 2, "high2")
	push(PriorityDefault, 3, "default1")
	push(PriorityLow, 4, "low")
	push(PriorityHigh, 1, "high1")
	push(PriorityDefault, 6, "default2")

	var got []string
	for h.Len() > 0 {
		got = append(got, heap.Pop(h).(*taskEntry).name)
	}
	want := []string{"high1", "high2", "default1", "default2", "low", "idle"}
	if len(got) != len(want) {
		t.Fatalf("pop order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pop order = %v, want %v", got, want)
		}
	}
}

// TestBackgroundExecutorPriorityOrdering saturates the pool and queues a
// mixed-priority batch, then verifies the deterministic observable: the
// first four tasks to leave the queue are exactly the four highest-priority
// ones (within-wave order is inherently nondeterministic across the four
// workers; exact priority+FIFO order is asserted by TestTaskHeapOrdering).
func TestBackgroundExecutorPriorityOrdering(t *testing.T) {
	be := NewBackgroundExecutor()
	_, release := gatePool(t, be)

	var mu sync.Mutex
	var arrivals int32
	var firstWave []string
	var all []string
	barrier := make(chan struct{})

	spawn := func(priority Priority, name string) {
		be.SpawnWithPriority(priority, func(context.Context) {
			mu.Lock()
			all = append(all, name)
			mu.Unlock()
			n := atomic.AddInt32(&arrivals, 1)
			if n <= 4 { // first four popped start first
				mu.Lock()
				firstWave = append(firstWave, name)
				mu.Unlock()
			}
			if n == 4 {
				close(barrier)
			}
			<-barrier
		})
	}
	spawn(PriorityHigh, "high1")
	spawn(PriorityHigh, "high2")
	spawn(PriorityDefault, "default1")
	spawn(PriorityDefault, "default2")
	spawn(PriorityLow, "low")
	spawn(PriorityIdle, "idle")

	close(release) // let the gates finish; workers drain the queue

	deadline := time.After(2 * time.Second)
	for {
		mu.Lock()
		n := len(all)
		mu.Unlock()
		if n == 6 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("tasks did not drain: %v", all)
		case <-time.After(5 * time.Millisecond):
		}
	}
	mu.Lock()
	defer mu.Unlock()
	set := map[string]bool{}
	for _, name := range firstWave {
		set[name] = true
	}
	want := map[string]bool{"high1": true, "high2": true, "default1": true, "default2": true}
	if len(firstWave) != 4 || !reflect.DeepEqual(set, want) {
		t.Fatalf("first wave = %v, want exactly %v", firstWave, want)
	}
}

// TestBackgroundTaskCancelBeforeRun cancels a queued task: it must be
// skipped entirely (the closure never runs) and Done must close.
func TestBackgroundTaskCancelBeforeRun(t *testing.T) {
	be := NewBackgroundExecutor()
	_, release := gatePool(t, be)

	ran := false
	task := be.SpawnWithPriority(PriorityDefault, func(context.Context) {
		ran = true
	})
	task.Cancel()
	close(release)
	waitDone(t, task)
	if ran {
		t.Fatal("cancelled task must not run")
	}
}

// TestBackgroundTaskCancelDuringRunCooperative cancels a RUNNING task: the
// spawned function observes cancellation through its context (cooperative,
// like upstream await points) and Done closes when it returns.
func TestBackgroundTaskCancelDuringRunCooperative(t *testing.T) {
	be := NewBackgroundExecutor()
	started := make(chan struct{})
	observed := make(chan error, 1)
	task := be.SpawnWithPriority(PriorityDefault, func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		observed <- ctx.Err()
	})
	<-started
	task.Cancel()
	select {
	case err := <-observed:
		if err != context.Canceled {
			t.Fatalf("ctx.Err() = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("task did not observe cancellation")
	}
	waitDone(t, task)
}

// TestBackgroundTaskDetach detaches a task and then cancels: the cancel is
// a no-op (detached tasks cannot be cancelled from the handle), so the
// closure still runs to completion (upstream Task::detach).
func TestBackgroundTaskDetach(t *testing.T) {
	be := NewBackgroundExecutor()
	_, release := gatePool(t, be)

	ran := false
	task := be.SpawnWithPriority(PriorityDefault, func(context.Context) {
		ran = true
	})
	task.Detach()
	task.Cancel()
	close(release)
	waitDone(t, task)
	if !ran {
		t.Fatal("detached task must run even after Cancel")
	}
}

// TestBackgroundTaskDetachRunningTask makes sure Detach on an already
// running task keeps Done well-defined (exactly one close).
func TestBackgroundTaskDetachRunningTask(t *testing.T) {
	be := NewBackgroundExecutor()
	started := make(chan struct{})
	task := be.SpawnWithPriority(PriorityDefault, func(ctx context.Context) {
		close(started)
	})
	task.Detach()
	<-started
	waitDone(t, task)
}

// ---------------------------------------------------------------------------
// Foreground tasks (upstream ForegroundExecutor::spawn's Task)
// ---------------------------------------------------------------------------

// TestForegroundTaskRuns verifies a spawned foreground closure runs on the
// next Pump and Done closes afterwards.
func TestForegroundTaskRuns(t *testing.T) {
	a := New()
	ran := false
	task := a.SpawnForeground(func(*App) { ran = true })
	a.Pump()
	select {
	case <-task.Done():
	case <-time.After(time.Second):
		t.Fatal("foreground task Done not closed")
	}
	if !ran {
		t.Fatal("foreground task did not run")
	}
}

// TestForegroundTaskCancelBeforeRun cancels before the queue drains: the
// closure must never run.
func TestForegroundTaskCancelBeforeRun(t *testing.T) {
	a := New()
	ran := false
	task := a.SpawnForeground(func(*App) { ran = true })
	task.Cancel()
	a.Pump()
	select {
	case <-task.Done():
	case <-time.After(time.Second):
		t.Fatal("cancelled foreground task Done not closed")
	}
	if ran {
		t.Fatal("cancelled foreground task must not run")
	}
}
