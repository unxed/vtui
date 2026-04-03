package vtui

import (
	"testing"
	"time"
)

func TestRunAsync_TaskExecutionAndCancellation(t *testing.T) {
	// Setup isolated FrameManager for the test
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
	fm.TaskChan = make(chan func(), 10)

	oldFm := FrameManager
	FrameManager = fm
	defer func() { FrameManager = oldFm }()

	// 1. Test Execution via RunOnUI
	done := make(chan bool, 1)
	ctx := RunAsync(func(c *TaskContext) {
		c.RunOnUI(func() {
			done <- true
		})
	})

	// Simulate main loop extracting the task
	select {
	case task := <-fm.TaskChan:
		task() // Execute the safe UI closure
	case <-time.After(1 * time.Second):
		t.Fatal("Task was not pushed to TaskChan")
	}

	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Error("RunOnUI task was not fully executed")
	}

	// 2. Test Cancellation
	ctx.Cancel()
	if ctx.Err() == nil {
		t.Error("TaskContext should report an error after Cancel() is called")
	}
}