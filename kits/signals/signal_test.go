package signals_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"testing"
	"time"

	sig "github.com/froppa/stackkit/kits/signals"
	"github.com/stretchr/testify/require"
)

func TestGracefulFastPath_NoForce(t *testing.T) {
	t.Parallel()

	var wg sync.WaitGroup
	s := sig.New(&wg)

	// No workers; graceful cancel should return immediately and never force.
	s.TriggerGraceful()
	start := time.Now()
	s.Wait(200 * time.Millisecond)

	require.NoError(t, s.Force().Err(), "force must not be canceled")
	require.Less(t, time.Since(start), 150*time.Millisecond)
}

func TestForcesAfterTimeout(t *testing.T) {
	t.Parallel()

	var wg sync.WaitGroup
	s := sig.New(&wg)

	wg.Add(1)
	done := make(chan struct{})
	go func() {
		defer wg.Done()
		<-s.Force().Done() // only exit on force
		close(done)
	}()

	s.TriggerGraceful()
	timeout := 60 * time.Millisecond
	start := time.Now()
	s.Wait(timeout)

	require.Error(t, s.Force().Err(), "force must be canceled after timeout")
	require.GreaterOrEqual(t, time.Since(start), timeout)

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("worker did not exit after force-cancel")
	}
}

func TestTriggerGraceful_Idempotent(t *testing.T) {
	t.Parallel()

	var wg sync.WaitGroup
	s := sig.New(&wg)

	wg.Add(1)
	exited := make(chan struct{})
	go func() {
		defer wg.Done()
		select {
		case <-s.Graceful().Done():
			close(exited)
		case <-s.Force().Done():
			close(exited)
		}
	}()

	// Call twice; should not panic or touch force.
	s.TriggerGraceful()
	s.TriggerGraceful()
	s.Wait(200 * time.Millisecond)

	require.NoError(t, s.Force().Err())
	select {
	case <-exited:
	case <-time.After(150 * time.Millisecond):
		t.Fatal("worker did not exit after graceful cancel")
	}
}

func TestNewWithSignals_UsesSubprocess(t *testing.T) {
	// Spawn a child that installs signal handler and reports via stdout.
	cmd := exec.Command(os.Args[0], "-test.run=TestSignalChildHelper", "--", "child")
	cmd.Env = append(os.Environ(), "RUN_SIGNAL_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("child failed: %v; out=%s", err, string(out))
	}
	require.Contains(t, string(out), "child:got-graceful")
}

// TestSignalChildHelper is invoked as a subprocess by TestNewWithSignals_UsesSubprocess.
func TestSignalChildHelper(t *testing.T) {
	if os.Getenv("RUN_SIGNAL_CHILD") != "1" {
		t.Skip("helper")
	}

	// Child process: set up handler, self-signal, observe graceful.
	var wg sync.WaitGroup
	s := sig.NewWithSignals(context.Background(), &wg)

	// Allow the signal goroutine time to register handlers before we send the
	// termination signal. Without this small delay the process may exit with the
	// default SIGTERM behaviour before the Notify call takes effect, which would
	// make the test flaky.
	time.Sleep(25 * time.Millisecond)

	// Send SIGTERM to self. Handler must consume it and cancel graceful.
	self := os.Getpid()
	if err := syscall.Kill(self, syscall.SIGTERM); err != nil {
		fmt.Fprintf(os.Stderr, "child:kill-err:%v\n", err)
		os.Exit(2)
	}

	select {
	case <-s.Graceful().Done():
		fmt.Fprintln(os.Stdout, "child:got-graceful") //nolint:errcheck
	case <-time.After(250 * time.Millisecond):
		fmt.Fprintln(os.Stderr, "child:timeout-waiting-graceful")
		os.Exit(3)
	}

	// Ensure Wait returns fast when graceful already canceled and no workers.
	start := time.Now()
	s.Wait(200 * time.Millisecond)
	if time.Since(start) > 150*time.Millisecond {
		fmt.Fprintln(os.Stderr, "child:wait-too-slow")
		os.Exit(4)
	}
}

func TestSecondSignal_ForcesShutdown(t *testing.T) {
	// This test runs a subprocess that should not exit when it receives a second
	// signal. Instead, the force-shutdown mechanism should be triggered.
	cmd := exec.Command(os.Args[0], "-test.run=TestSecondSignalChildHelper", "--", "child")
	cmd.Env = append(os.Environ(), "RUN_SECOND_SIGNAL_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("child failed: %v; out=%s", err, string(out))
	}
	require.Contains(t, string(out), "child:got-force")
}

// TestSecondSignalChildHelper is invoked as a subprocess by TestSecondSignal_ForcesShutdown.
func TestSecondSignalChildHelper(t *testing.T) {
	if os.Getenv("RUN_SECOND_SIGNAL_CHILD") != "1" {
		t.Skip("helper")
	}

	var wg sync.WaitGroup
	s := sig.NewWithSignals(context.Background(), &wg)

	wg.Add(1)
	go func() {
		defer wg.Done()
		// This worker will only exit when the force context is canceled.
		<-s.Force().Done()
		fmt.Fprintln(os.Stdout, "child:got-force") //nolint:errcheck
	}()

	// Give the signal handler goroutine time to start.
	time.Sleep(25 * time.Millisecond)

	self := os.Getpid()
	// Send the first signal to trigger graceful shutdown.
	if err := syscall.Kill(self, syscall.SIGTERM); err != nil {
		fmt.Fprintf(os.Stderr, "child:kill-err:%v\n", err)
		os.Exit(2)
	}

	// Send a second signal immediately. The bug would cause the process to
	// exit here. The correct behavior is for the signal handler to swallow
	// this signal.
	if err := syscall.Kill(self, syscall.SIGTERM); err != nil {
		fmt.Fprintf(os.Stderr, "child:kill-err:%v\n", err)
		os.Exit(2)
	}

	// The Wait call will time out and trigger the force shutdown.
	s.Wait(100 * time.Millisecond)
}

func TestWaitGroupAccessorAndIntegration(t *testing.T) {
	t.Parallel()

	var wg sync.WaitGroup
	s := sig.New(&wg)

	// Accessor returns the original pointer
	require.Same(t, &wg, s.WaitGroup())

	// Worker exits shortly after graceful is triggered
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-s.Graceful().Done()
		time.Sleep(20 * time.Millisecond)
	}()

	s.TriggerGraceful()
	start := time.Now()
	s.Wait(200 * time.Millisecond)

	// Should complete well before timeout and without forcing
	require.NoError(t, s.Force().Err())
	require.Less(t, time.Since(start), 150*time.Millisecond)
}

func TestWait_ZeroTimeoutForcesImmediately(t *testing.T) {
	t.Parallel()

	var wg sync.WaitGroup
	s := sig.New(&wg)

	// Worker exits only on force
	wg.Add(1)
	done := make(chan struct{})
	go func() {
		defer wg.Done()
		<-s.Force().Done()
		close(done)
	}()

	s.TriggerGraceful()
	s.Wait(0)

	require.Error(t, s.Force().Err(), "force should be canceled immediately with zero timeout")

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("worker did not exit after immediate force-cancel")
	}
}

func TestParentContextCancelCancelsBoth(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	s := sig.NewWithSignals(ctx, &wg)

	// Cancel parent; both contexts should be done
	cancel()

	select {
	case <-s.Graceful().Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("graceful not canceled by parent cancel")
	}
	select {
	case <-s.Force().Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("force not canceled by parent cancel")
	}

	// Wait should return immediately (no workers), and force already canceled
	start := time.Now()
	s.Wait(200 * time.Millisecond)
	require.Less(t, time.Since(start), 100*time.Millisecond)
}

func TestGracefulThenSignalIgnored_NoForce(t *testing.T) {
	// Ensure that after graceful is canceled programmatically, a subsequent OS
	// signal is effectively ignored by the handler loop (which exits on
	// graceful Done) and does not flip the force context.
	var wg sync.WaitGroup
	s := sig.NewWithSignals(context.Background(), &wg)

	// Give the signal handler goroutine time to start.
	time.Sleep(25 * time.Millisecond)

	// Cancel graceful first
	s.TriggerGraceful()

	// Send SIGTERM after graceful is already canceled; should be ignored.
	_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)

	// Wait should return quickly (no workers) and without forcing
	start := time.Now()
	s.Wait(200 * time.Millisecond)
	require.Less(t, time.Since(start), 150*time.Millisecond)
	require.NoError(t, s.Force().Err(), "force must remain not canceled")
}

func TestWait_AwaitsMultipleWorkersWithinTimeout(t *testing.T) {
	t.Parallel()

	var wg sync.WaitGroup
	s := sig.New(&wg)

	wg.Add(2)
	go func() {
		defer wg.Done()
		<-s.Graceful().Done()
		time.Sleep(20 * time.Millisecond)
	}()
	go func() {
		defer wg.Done()
		<-s.Graceful().Done()
		time.Sleep(40 * time.Millisecond)
	}()

	s.TriggerGraceful()
	start := time.Now()
	s.Wait(200 * time.Millisecond)

	// Should complete before timeout and without forcing
	require.NoError(t, s.Force().Err())
	require.Less(t, time.Since(start), 150*time.Millisecond)
}
