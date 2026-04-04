package piecetable

import (
	"testing"
	"reflect"
)

func TestPieceTable_Basic(t *testing.T) {
	pt := New([]byte("Hello"))

	if pt.String() != "Hello" {
		t.Errorf("Expected 'Hello', got '%s'", pt.String())
	}
	if pt.Size() != 5 {
		t.Errorf("Expected size 5, got %d", pt.Size())
	}
}

func TestPieceTable_Insert(t *testing.T) {
	pt := New([]byte("Hello"))

	// Insert at end (Append)
	pt.Insert(5, []byte(" World"))
	if pt.String() != "Hello World" {
		t.Errorf("Insert end failed: %s", pt.String())
	}

	// Addition optimization: adding character at end, pieces should merge
	pt.Insert(11, []byte("!"))
	if pt.String() != "Hello World!" {
		t.Errorf("Insert optimization failed: %s", pt.String())
	}
	// We should have exactly 2 pieces: [Hello] and [ World!]
	if len(pt.pieces) != 2 {
		t.Errorf("Optimization failed, expected 2 pieces, got %d", len(pt.pieces))
	}

	// Insert at start
	pt.Insert(0, []byte("Say "))
	if pt.String() != "Say Hello World!" {
		t.Errorf("Insert start failed: %s", pt.String())
	}

	// Insert in middle (splitting original buffer)
	pt.Insert(6, []byte("o "))
	if pt.String() != "Say Heo llo World!" {
		t.Errorf("Insert middle failed: %s", pt.String())
	}
}

func TestPieceTable_Delete(t *testing.T) {
	pt := New([]byte("Hello World!"))

	// Deleting from middle of one piece
	pt.Delete(5, 6) // Remove " World"
	if pt.String() != "Hello!" {
		t.Errorf("Delete middle failed: %s", pt.String())
	}
	// After middle deletion, 1 piece should become 2
	if len(pt.pieces) != 2 {
		t.Errorf("Expected 2 pieces after middle delete, got %d", len(pt.pieces))
	}

	// Deleting on boundary (capturing end of left and start of right piece)
	pt.Insert(5, []byte(" World")) // Restored: "Hello World!" -> pieces: ["Hello"], [" World"], ["!"]

	pt.Delete(4, 3) // Remove "o W" -> Should leave "Hellorld!"
	if pt.String() != "Hellorld!" {
		t.Errorf("Delete across boundary failed: %s", pt.String())
	}

	// Deleting all text
	pt.Delete(0, pt.Size())
	if pt.String() != "" {
		t.Errorf("Delete all failed: '%s'", pt.String())
	}
	if pt.Size() != 0 {
		t.Errorf("Expected size 0, got %d", pt.Size())
	}
}

func TestPieceTable_Complex(t *testing.T) {
	pt := New([]byte("The quick brown fox jumps over the lazy dog"))

	pt.Delete(16, 4) // "The quick brown jumps over the lazy dog"
	pt.Insert(16, []byte("cat ")) // "The quick brown cat jumps over the lazy dog"
	pt.Delete(0, 4) // "quick brown cat jumps over the lazy dog"
	pt.Insert(pt.Size(), []byte(".")) // "quick brown cat jumps over the lazy dog."

	expected := "quick brown cat jumps over the lazy dog."
	if pt.String() != expected {
		t.Errorf("Complex test failed:\nExpected: %s\nGot:      %s", expected, pt.String())
	}
}

func TestPieceTable_GetRange(t *testing.T) {
	pt := New([]byte("0123456789"))
	pt.Insert(5, []byte("abc")) // "01234abc56789"

	// 1. Range from original buffer
	r1, _ := pt.GetRange(1, 3)
	if string(r1) != "123" {
		t.Error("GetRange failed on original buffer")
	}

	// 2. Range from add buffer
	r2, _ := pt.GetRange(6, 1)
	if string(r2) != "b" {
		t.Error("GetRange failed on add buffer")
	}

	// 3. Range spanning multiple pieces
	r3, _ := pt.GetRange(4, 4)
	if string(r3) != "4abc" {
		t.Error("GetRange failed on spanning pieces")
	}

	// 4. Edge cases
	r4, _ := pt.GetRange(0, pt.Size())
	if string(r4) != "01234abc56789" {
		t.Error("GetRange failed on full range")
	}
	rErr1, _ := pt.GetRange(-1, 5)
	rErr2, _ := pt.GetRange(0, 100)
	if rErr1 != nil || rErr2 != nil {
		t.Error("GetRange should return nil for invalid ranges")
	}
}
func TestPieceTable_MergeOptimization(t *testing.T) {
	pt := New([]byte("Start"))

	// 1. Insert at end (creates Add piece 1)
	pt.Insert(pt.Size(), []byte(" One"))
	if len(pt.pieces) != 2 {
		t.Fatalf("Expected 2 pieces, got %d", len(pt.pieces))
	}

	// 2. Insert at end again (should merge into Add piece 1)
	pt.Insert(pt.Size(), []byte(" Two"))
	if len(pt.pieces) != 2 {
		t.Errorf("Optimization failed: expected pieces to merge, got %d pieces", len(pt.pieces))
	}

	// 3. Insert in middle.
	// Note: Offset 5 is exactly between 'Start' and ' One Two'.
	// PieceTable inserts between pieces without splitting if offset is on boundary.
	pt.Insert(5, []byte(" Middle"))
	if len(pt.pieces) != 3 {
		t.Errorf("Expected 3 pieces after boundary insert, got %d", len(pt.pieces))
	}

	expected := "Start Middle One Two"
	if pt.String() != expected {
		t.Errorf("Data corrupted during merge test. Expected %q, got %q", expected, pt.String())
	}
}

func TestPieceTable_AppendRange_Boundary(t *testing.T) {
	pt := New([]byte("0123456789"))
	pt.Insert(5, []byte("XXX")) // 01234 XXX 56789

	dest := make([]byte, 0, 10)

	// Read across all three pieces: "4" (Orig), "XXX" (Add), "5" (Orig)
	dest, err := pt.AppendRange(dest, 4, 5)
	if err != nil {
		t.Fatalf("AppendRange failed: %v", err)
	}

	if string(dest) != "4XXX5" {
		t.Errorf("AppendRange across boundaries failed. Expected '4XXX5', got %q", string(dest))
	}

	// Ensure no data was overwritten improperly
	dest = append(dest, []byte("Tail")...)
	if string(dest) != "4XXX5Tail" {
		t.Errorf("AppendRange modified slice capacity/length improperly: %q", string(dest))
	}
}
func TestPieceTable_EmptyOperations(t *testing.T) {
	pt := New([]byte("abc"))

	// 1. Zero length insert
	pt.Insert(1, []byte(""))
	if pt.Size() != 3 || pt.String() != "abc" {
		t.Error("Zero length insert modified data")
	}

	// 2. Zero length delete
	pt.Delete(1, 0)
	if pt.Size() != 3 || pt.String() != "abc" {
		t.Error("Zero length delete modified data")
	}

	// 3. Out of bounds delete
	pt.Delete(1, 10)
	if pt.Size() != 3 {
		t.Error("Out of bounds delete should be ignored")
	}
}

func TestPieceTable_BoundaryInsert(t *testing.T) {
	// Original: [AA][BB]
	pt := New([]byte("AABB"))
	// Insert at 2 (between AA and BB)
	pt.Insert(2, []byte("XX"))
	// Now pieces: [AA][XX][BB]

	// Delete [XX] exactly
	pt.Delete(2, 2)

	if pt.String() != "AABB" {
		t.Errorf("Boundary delete failed, got %q", pt.String())
	}
	if len(pt.pieces) != 2 {
		t.Errorf("Expected pieces to collapse/stay clean, got %d pieces", len(pt.pieces))
	}
}
func TestPieceTable_FragmentationStress(t *testing.T) {
	// Create a document and perform many tiny operations to force piece splitting.
	pt := New([]byte("INITIAL"))

	// Interleaved inserts
	for i := 0; i < 100; i++ {
		pt.Insert(pt.Size()/2, []byte("x"))
	}

	// Interleaved deletes
	for i := 0; i < 50; i++ {
		pt.Delete(i, 1)
	}

	expectedLen := 7 + 100 - 50
	if pt.Size() != expectedLen {
		t.Errorf("Stress size mismatch: expected %d, got %d", expectedLen, pt.Size())
	}

	// Verify we can still read the whole document without errors
	_, err := pt.Bytes()
	if err != nil {
		t.Errorf("Fragmentation caused corruption: %v", err)
	}
}
func TestPieceTable_StreamingIntegrity(t *testing.T) {
	// Tests ForEachRange which is used for saving files to disk.
	// We need to ensure it yields exactly the same bytes as a full memory dump.
	content := "The quick brown fox jumps over the lazy dog"
	pt := New([]byte(content))

	// Fragment the table with multiple operations
	pt.Delete(4, 6)               // "The brown fox..."
	pt.Insert(4, []byte("lazy ")) // "The lazy brown fox..."
	pt.Insert(pt.Size(), []byte("!"))

	memBytes, _ := pt.Bytes()
	var streamBytes []byte

	err := pt.ForEachRange(func(data []byte) error {
		streamBytes = append(streamBytes, data...)
		return nil
	})

	if err != nil {
		t.Fatalf("ForEachRange failed: %v", err)
	}

	if !reflect.DeepEqual(memBytes, streamBytes) {
		t.Errorf("Streaming integrity failed.\nMem:    %q\nStream: %q", string(memBytes), string(streamBytes))
	}

	if string(streamBytes) != "The lazy brown fox jumps over the lazy dog!" {
		t.Errorf("Resulting text is wrong: %q", string(streamBytes))
	}
}
func TestPieceTable_ExtremeFragmentation(t *testing.T) {
	// Forces hundreds of pieces and tests reading across many of them.
	pt := New([]byte("A"))
	expected := "A"

	// 1. Create many pieces via interleaved insertions.
	// We insert at position 0 to avoid the 'append-at-end' merge optimization.
	for i := 0; i < 500; i++ {
		pt.Insert(0, []byte("X"))
		expected = "X" + expected
	}

	if len(pt.pieces) < 500 {
		t.Errorf("Expected fragmentation, got %d pieces", len(pt.pieces))
	}

	// 2. Test GetRange spanning many pieces
	// Read everything except the first and last chars
	res, _ := pt.GetRange(1, 499)
	if len(res) != 499 {
		t.Errorf("GetRange length mismatch: expected 499, got %d", len(res))
	}

	// 3. Test multi-piece deletion
	// Remove middle 400 'X' chars
	pt.Delete(50, 400)
	if pt.Size() != 101 {
		t.Errorf("Size after multi-piece delete mismatch: expected 101, got %d", pt.Size())
	}

	// Verify content remains valid
	if pt.String() != expected[:50]+expected[450:] {
		t.Error("Content corrupted after multi-piece delete")
	}
}
func TestPieceTable_ReadAtBoundary(t *testing.T) {
	// Specifically targets the logic that stitches data from multiple pieces.
	// Piece 1: [0..9] (Original), Piece 2: [10..14] (Add), Piece 3: [15..24] (Original)
	pt := New([]byte("012345678956789")) // "0123456789" then "56789"
	pt.Insert(10, []byte("ABCDE"))        // Result: "0123456789ABCDE56789"

	// Pieces are:
	// 0: Original, Start 0, Len 10 ("0123456789")
	// 1: Add,      Start 0, Len 5  ("ABCDE")
	// 2: Original, Start 10, Len 5 ("56789")

	tests := []struct {
		off, len int
		expected string
	}{
		{9, 2, "9A"},   // Spans Piece 0 and 1
		{14, 2, "E5"},  // Spans Piece 1 and 2
		{10, 5, "ABCDE"}, // Exactly Piece 1
		{0, 20, "0123456789ABCDE56789"}, // All pieces
		{8, 9, "89ABCDE56"}, // Spans all three pieces
	}

	for _, tt := range tests {
		data, err := pt.GetRange(tt.off, tt.len)
		if err != nil {
			t.Errorf("GetRange(%d, %d) error: %v", tt.off, tt.len, err)
			continue
		}
		if string(data) != tt.expected {
			t.Errorf("GetRange(%d, %d): expected %q, got %q", tt.off, tt.len, tt.expected, string(data))
		}

		// Also test AppendRange (zero-allocation variant)
		buf := make([]byte, 0, tt.len)
		buf, _ = pt.AppendRange(buf, tt.off, tt.len)
		if string(buf) != tt.expected {
			t.Errorf("AppendRange(%d, %d) failed", tt.off, tt.len)
		}
	}
}

func TestPieceTable_MixedBuffer_ComplexRead(t *testing.T) {
	// Create a piece table with highly interleaved pieces from both buffers
	// [Orig:0-2][Add:0-2][Orig:2-4][Add:2-4]
	// "AB" + "12" + "CD" + "34" = "AB12CD34"
	pt := New([]byte("ABCD"))
	pt.Insert(2, []byte("12")) // "AB12CD"
	pt.Insert(6, []byte("34")) // "AB12CD34"

	tests := []struct {
		off, len int
		want     string
	}{
		{0, 8, "AB12CD34"}, // Full
		{1, 4, "B12C"},     // Crosses 3 pieces
		{3, 2, "2C"},       // Crosses Add and Orig
		{7, 1, "4"},        // Tail
		{0, 0, ""},         // Zero length
	}

	for _, tt := range tests {
		got, err := pt.GetRange(tt.off, tt.len)
		if err != nil { t.Errorf("GetRange(%d,%d) err: %v", tt.off, tt.len, err) }
		if string(got) != tt.want {
			t.Errorf("GetRange(%d,%d) = %q, want %q", tt.off, tt.len, string(got), tt.want)
		}
	}
}


func TestPieceTable_Delete_PieceRemoval(t *testing.T) {
	// Verifies that deleting a range that exactly matches one or more pieces
	// correctly removes them from the table.
	pt := New([]byte("AAA"))
	pt.Insert(0, []byte("CCC")) // [CCC][AAA]
	pt.Insert(3, []byte("BBB")) // [CCC][BBB][AAA]

	// We inserted at 0 and then in the middle, preventing the "append-at-end" merge optimization.
	if len(pt.pieces) != 3 { t.Fatalf("Expected 3 pieces, got %d", len(pt.pieces)) }

	// Delete "BBB" (offset 3, length 3)
	pt.Delete(3, 3)

	if pt.String() != "CCCAAA" { t.Errorf("Delete failed: %q", pt.String()) }
	if len(pt.pieces) != 2 {
		t.Errorf("Piece was not removed from table, count: %d", len(pt.pieces))
	}
}
