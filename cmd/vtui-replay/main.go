package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"reflect"
	"time"

	"github.com/unxed/vtui"
)

type RecordEntry struct {
	Time float64        `json:"time"`
	Dir  string         `json:"dir"` // "down" | "up"
	Msg  map[string]any `json:"msg"`
}

func main() {
	verifyFlag := flag.Bool("verify", true, "Verify kernel responses against recorded output")
	speedFlag := flag.Float64("speed", 0.0, "Playback speed multiplier (0 = immediate)")
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: vtui-replay [options] <session.jsonl>")
		os.Exit(1)
	}

	recordPath := args[0]
	f, err := os.Open(recordPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "vtui-replay: cannot open %s: %v\n", recordPath, err)
		os.Exit(1)
	}
	defer f.Close()

	if err := replaySession(f, *verifyFlag, *speedFlag); err != nil {
		fmt.Fprintf(os.Stderr, "vtui-replay failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Replay completed successfully.")
}

func replaySession(r io.Reader, verify bool, speed float64) error {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)

	fm := vtui.NewFrameManager()
	fm.Init(scr)
	fm.Push(vtui.NewDesktop())
	defer fm.Shutdown()
	fm.SetHostMode(true)
	runDone := make(chan struct{})
	go func() {
		fm.Run()
		close(runDone)
	}()

	clientReader, serverWriter := io.Pipe()
	serverReader, clientWriter := io.Pipe()

	session := vtui.NewProtocolSession(serverReader, serverWriter, fm)
	serveDone := make(chan struct{})
	go func() {
		_ = session.Serve()
		close(serveDone)
	}()
	defer func() {
		_ = clientWriter.Close()
		<-serveDone
		_ = serverWriter.Close()
		<-runDone
	}()

	scanner := bufio.NewScanner(r)
	serverScanner := bufio.NewScanner(clientReader)

	lastTime := 0.0
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry RecordEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return fmt.Errorf("malformed record line: %w", err)
		}

		if speed > 0 && entry.Time > lastTime {
			sleepDur := time.Duration((entry.Time - lastTime) / speed * float64(time.Second))
			time.Sleep(sleepDur)
		}
		lastTime = entry.Time

		if entry.Dir == "down" {
			data, _ := json.Marshal(entry.Msg)
			_, _ = clientWriter.Write(append(data, '\n'))
		} else if entry.Dir == "up" && verify {
			if !serverScanner.Scan() {
				return fmt.Errorf("expected response from kernel for %v, got EOF", entry.Msg)
			}
			var gotMsg map[string]any
			if err := json.Unmarshal(serverScanner.Bytes(), &gotMsg); err != nil {
				return fmt.Errorf("kernel emitted invalid JSON: %w", err)
			}

			// Ignore non-deterministic runtime fields like features/backend names in equality
			if entry.Msg["op"] == "welcome" && gotMsg["op"] == "welcome" {
				continue
			}

			if !reflect.DeepEqual(entry.Msg, gotMsg) {
				return fmt.Errorf("response mismatch:\nwant: %v\ngot:  %v", entry.Msg, gotMsg)
			}
		}
	}

	return scanner.Err()
}
