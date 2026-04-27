package vtui

import (
	"context"
	"fmt"
	"os"
)

// TaskContext provides a safe environment for background operations
// to interact with the main UI thread.
type TaskContext struct {
	context.Context
	Cancel context.CancelFunc
}

// RunOnUI safely executes the given function on the main UI thread.
// This MUST be used for any updates to ScreenObjects (changing text, showing dialogs).
func (ctx *TaskContext) RunOnUI(fn func()) {
	FrameManager.PostTask(fn)
}

// RunAsync starts a background goroutine and provides it with a TaskContext.
// This is the foundation for background plugins, VFS operations, and heavy logic.
func RunAsync(worker func(ctx *TaskContext)) *TaskContext {
	ctx, cancel := context.WithCancel(context.Background())
	taskCtx := &TaskContext{
		Context: ctx,
		Cancel:  cancel,
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				DebugLog("FATAL PANIC IN ASYNC TASK: %v", r)
				crashPath := RecordCrash(r, nil)
				Suspend()
				CleanupStderrLog()
				fmt.Fprintf(os.Stderr, "\n[%s] FATAL PANIC IN ASYNC TASK: %v\n", AppName, r)
				if crashPath != "" {
					fmt.Fprintf(os.Stderr, "[%s] Crash report saved to: %s\n", AppName, crashPath)
				}
				os.Exit(2)
			}
		}()
		worker(taskCtx)
	}()

	return taskCtx
}