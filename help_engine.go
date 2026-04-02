package vtui

import (
	"bufio"
	"context"
	"io"
	"strings"
)

// HelpVFS is a minimal interface needed by HelpEngine to load files.
// This prevents circular dependencies on the main vfs package.
type HelpVFS interface {
	Open(ctx context.Context, path string) (io.ReadCloser, error)
}

// HelpLink represents a hyperlink within a help topic.
type HelpLink struct {
	Text   string
	Target string
	Line   int
	X1, X2 int
}

// HelpTopic contains formatted lines and metadata for a single help page.
type HelpTopic struct {
	Name       string
	Lines      []string
	Links      []HelpLink
	StickyRows int // Number of lines from the top that don't scroll ($ syntax)
}

// HelpEngine manages loading and parsing of help files.
type HelpEngine struct {
	vfs    HelpVFS
	topics map[string]*HelpTopic
}

// ctxReader adapts context-aware reader to standard io.Reader.
type ctxReader struct {
	ctx context.Context
	r   io.ReadCloser
}

func (cr *ctxReader) Read(p []byte) (int, error) {
	select {
	case <-cr.ctx.Done():
		return 0, cr.ctx.Err()
	default:
		return cr.r.Read(p)
	}
}
// GlobalHelpEngine is the default engine used by the framework for F1 lookups.
var GlobalHelpEngine *HelpEngine

func NewHelpEngine(v HelpVFS) *HelpEngine {
	return &HelpEngine{
		vfs:    v,
		topics: make(map[string]*HelpTopic),
	}
}

// LoadFile reads an .hlf file and populates the topic cache.
func (e *HelpEngine) LoadFile(path string) error {
	ctx := context.Background()
	f, err := e.vfs.Open(ctx, path)
	if err != nil {
		return err
	}
	defer f.Close()

	// Wrap our context-aware reader into a standard io.Reader for bufio
	scanner := bufio.NewScanner(&ctxReader{ctx, f})
	var currentTopic *HelpTopic

	lineIdx := 0
	for scanner.Scan() {
		line := scanner.Text()

		// 1. Topic Definition: @TopicName
		if strings.HasPrefix(line, "@") && !strings.Contains(line, "~") {
			name := strings.TrimPrefix(line, "@")
			currentTopic = &HelpTopic{Name: name}
			e.topics[name] = currentTopic
			lineIdx = 0
			continue
		}

		if currentTopic == nil {
			continue
		}

		// 2. Sticky Header: $Header
		if strings.HasPrefix(line, "$") && lineIdx == currentTopic.StickyRows {
			currentTopic.StickyRows++
			currentTopic.Lines = append(currentTopic.Lines, line[1:])
			lineIdx++
			continue
		}

		// 3. Normal line
		currentTopic.Lines = append(currentTopic.Lines, line)
		e.parseLinks(currentTopic, line, lineIdx)
		lineIdx++
	}
	return nil
}

// parseLinks extracts ~Text~Target@ patterns from a line.
func (e *HelpEngine) parseLinks(topic *HelpTopic, line string, lineIdx int) {
	// Simple state machine for ~LinkText~Target@
	start := -1
	for i := 0; i < len(line); i++ {
		if line[i] == '~' {
			if start == -1 {
				start = i
			} else {
				// Find target end
				targetStart := i + 1
				atIdx := strings.Index(line[targetStart:], "@")
				if atIdx != -1 {
					atIdx += targetStart
					linkText := line[start+1 : i]
					target := line[targetStart:atIdx]

					// Calculate visual positions (simplified, assuming 1 char = 1 cell for now)
					// In the real view, we will need a more precise runewidth calculation.
					topic.Links = append(topic.Links, HelpLink{
						Text:   linkText,
						Target: target,
						Line:   lineIdx,
						X1:     start, // This is raw, will be refined in HelpView
						X2:     atIdx,
					})
					i = atIdx
					start = -1
				} else {
					start = -1 // Malformed link
				}
			}
		}
	}
}

func (e *HelpEngine) GetTopic(name string) *HelpTopic {
	return e.topics[name]
}