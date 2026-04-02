package vtui

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type mockHelpVFS struct{}

func (m *mockHelpVFS) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	return os.Open(path)
}

func TestHelpEngine_Parsing(t *testing.T) {
	// Create dummy help content
	tmpDir := t.TempDir()
	helpPath := filepath.Join(tmpDir, "test.hlf")

	content := `
@Contents
$Manual Header
This is a #bold# word.
	See ~Introduction~IntroTopic@ for details.
  ^Centered line

@IntroTopic
$Introduction
Welcome to the intro.
`
	os.WriteFile(helpPath, []byte(content), 0644)

	engine := NewHelpEngine(&mockHelpVFS{})
	err := engine.LoadFile(helpPath)
	if err != nil {
		t.Fatalf("Failed to load help file: %v", err)
	}

	// 1. Test topic extraction
	contents := engine.GetTopic("Contents")
	if contents == nil { t.Fatal("Topic 'Contents' not found") }

	// 2. Test sticky headers
	if contents.StickyRows != 1 {
		t.Errorf("Expected 1 sticky row, got %d", contents.StickyRows)
	}
	if contents.Lines[0] != "Manual Header" {
		t.Errorf("Header content mismatch: %q", contents.Lines[0])
	}

	// 3. Test link extraction
	if len(contents.Links) != 1 {
		t.Fatalf("Expected 1 link, got %d", len(contents.Links))
	}
	link := contents.Links[0]
	if link.Text != "Introduction" || link.Target != "IntroTopic" {
		t.Errorf("Link data mismatch: %+v", link)
	}
	if link.Line != 2 { // Line 0 is header, 1 is text, 2 is link
		t.Errorf("Link line mismatch: expected 2, got %d", link.Line)
	}

	// 4. Test multiple topics
	intro := engine.GetTopic("IntroTopic")
	if intro == nil || intro.Lines[1] != "Welcome to the intro." {
		t.Error("IntroTopic parsing failed")
	}
}

func TestHelpEngine_Parsing_Complex(t *testing.T) {
	engine := NewHelpEngine(&mockHelpVFS{})

	// Test multiple links on one line and nested formatting
	topic := &HelpTopic{Name: "Test"}
	line := "See ~Link 1~T1@ and ~Link 2~T2@. Also #~Link In Bold~T3@#"
	engine.parseLinks(topic, line, 0)

	if len(topic.Links) != 3 {
		t.Fatalf("Expected 3 links, got %d", len(topic.Links))
	}
	if topic.Links[0].Text != "Link 1" || topic.Links[1].Target != "T2" {
		t.Error("Link text or target extraction failed")
	}
	if topic.Links[2].Text != "Link In Bold" {
		t.Error("Link inside bold markers failed")
	}
}
