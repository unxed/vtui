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

	// frames is the frame manager this task was started against, remembered
	// so the task keeps posting to it even if the global is replaced while it
	// runs. Zero-valued TaskContexts built by callers leave this nil and fall
	// back to the global, as before.
	frames *FrameManagerType
}

// RunOnUI safely executes the given function on the main UI thread.
// This MUST be used for any updates to ScreenObjects (changing text, showing dialogs).
func (ctx *TaskContext) RunOnUI(fn func()) {
	// Reading the global here would mean reading it from the background
	// goroutine, racing anything that assigns to FrameManager while the task
	// is still in flight. A task belongs to the manager it was started
	// against, so use that one when it is known.
	if fm := ctx.frames; fm != nil {
		fm.PostTask(fn)
		return
	}
	FrameManager.PostTask(fn)
}

// RunAsync starts a background goroutine and provides it with a TaskContext.
// This is the foundation for background plugins, VFS operations, and heavy logic.
func RunAsync(worker func(ctx *TaskContext)) *TaskContext {
	ctx, cancel := context.WithCancel(context.Background())
	taskCtx := &TaskContext{
		Context: ctx,
		Cancel:  cancel,
		// Read on the goroutine that starts the task, not on the one that
		// runs it.
		frames: FrameManager,
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				DebugLog("FATAL PANIC IN ASYNC TASK: %v", r)
				crashPath := RecordCrash(r, nil)
				Suspend()
				fmt.Fprintf(os.Stderr, "\n[%s] FATAL PANIC IN ASYNC TASK: %v\n", AppName, r)
				if crashPath != "" {
					fmt.Fprintf(os.Stderr, "[%s] Crash report saved to: %s\n", AppName, crashPath)
				}
				CleanupStderrLog()
				os.Exit(2)
			}
		}()
		worker(taskCtx)
	}()

	return taskCtx
}
