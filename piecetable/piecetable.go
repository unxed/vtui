package piecetable

// BufferType indicates which buffer a text fragment is in.
import "errors"

var ErrLoading = errors.New("data is loading")

// BufferType indicates which buffer a text fragment is in.
type Buffer interface {
	Size() int
	Read(offset, length int) ([]byte, error)
}

type MemoryBuffer []byte

func (m MemoryBuffer) Size() int { return len(m) }
func (m MemoryBuffer) Read(offset, length int) ([]byte, error) {
	if offset < 0 || offset >= len(m) || length <= 0 {
		return nil, nil
	}
	end := offset + length
	if end > len(m) {
		end = len(m)
	}
	return m[offset:end], nil
}
type BufferType int

const (
	Original BufferType = iota
	Add
)

// Piece describes one text fragment.
type Piece struct {
	Buf    BufferType
	Start  int // Offset of the fragment start in the corresponding buffer
	Length int // Piece length
}

// PieceTable is a structure for efficient editing of large texts.
type PieceTable struct {
	orig   Buffer  // Original (Read-only) buffer
	add    []byte  // Additive (Append-only) buffer
	pieces []Piece // Piece table
	size   int     // Current logical length of the entire text
}

// New creates a new piece table from original text.
func New(text []byte) *PieceTable {
	pt := &PieceTable{
		orig: MemoryBuffer(text),
		size: len(text),
	}
	if len(text) > 0 {
		pt.pieces = []Piece{{Buf: Original, Start: 0, Length: len(text)}}
	}
	return pt
}

// Size returns current logical length of the text.
func (pt *PieceTable) Size() int {
	return pt.size
}
// GetOriginalBuffer returns the underlying original buffer.
func (pt *PieceTable) GetOriginalBuffer() Buffer {
	return pt.orig
}

// offsetToPiece finds piece index and offset within it by global offset.
func (pt *PieceTable) offsetToPiece(offset int) (pieceIdx int, offsetInPiece int) {
	if offset == pt.size {
		return len(pt.pieces), 0
	}
	curr := 0
	for i, p := range pt.pieces {
		if offset < curr+p.Length {
			return i, offset - curr
		}
		curr += p.Length
	}
	return len(pt.pieces), 0
}

// Insert inserts data at the specified offset.
func (pt *PieceTable) Insert(offset int, data []byte) {
	if offset < 0 || offset > pt.size || len(data) == 0 {
		return
	}

	addStart := len(pt.add)
	pt.add = append(pt.add, data...)
	newPiece := Piece{Buf: Add, Start: addStart, Length: len(data)}

	// If the table is empty
	if pt.size == 0 {
		pt.pieces = []Piece{newPiece}
		pt.size += len(data)
		return
	}

	// Optimization: if inserting at the very end and previous piece is also Add — merge them
	if offset == pt.size && len(pt.pieces) > 0 {
		lastIdx := len(pt.pieces) - 1
		lastP := pt.pieces[lastIdx]
		if lastP.Buf == Add && lastP.Start+lastP.Length == addStart {
			pt.pieces[lastIdx].Length += len(data)
			pt.size += len(data)
			return
		}
		// Otherwise just append a new piece to the end
		pt.pieces = append(pt.pieces, newPiece)
		pt.size += len(data)
		return
	}

	// General case: insertion in the middle
	idx, off := pt.offsetToPiece(offset)
	p := pt.pieces[idx]

	var newPieces []Piece
	newPieces = append(newPieces, pt.pieces[:idx]...)

	if off == 0 {
		// Insertion exactly before the piece
		newPieces = append(newPieces, newPiece, p)
	} else {
		// Split the current piece into two
		left := Piece{Buf: p.Buf, Start: p.Start, Length: off}
		right := Piece{Buf: p.Buf, Start: p.Start + off, Length: p.Length - off}
		newPieces = append(newPieces, left, newPiece, right)
	}

	if idx+1 < len(pt.pieces) {
		newPieces = append(newPieces, pt.pieces[idx+1:]...)
	}

	pt.pieces = newPieces
	pt.size += len(data)
}

// Delete removes a text fragment of specified length starting from offset.
func (pt *PieceTable) Delete(offset, length int) {
	if offset < 0 || length <= 0 || offset+length > pt.size {
		return
	}

	startIdx, startOff := pt.offsetToPiece(offset)
	endIdx, endOff := pt.offsetToPiece(offset + length)

	var newPieces []Piece
	newPieces = append(newPieces, pt.pieces[:startIdx]...)

	// Remainder of the left split piece
	if startOff > 0 {
		p := pt.pieces[startIdx]
		newPieces = append(newPieces, Piece{Buf: p.Buf, Start: p.Start, Length: startOff})
	}

	// Remainder of the right split piece
	if endIdx < len(pt.pieces) {
		p := pt.pieces[endIdx]
		if endOff < p.Length {
			newPieces = append(newPieces, Piece{Buf: p.Buf, Start: p.Start + endOff, Length: p.Length - endOff})
		}
	}

	// All pieces after endIdx
	if endIdx+1 < len(pt.pieces) {
		newPieces = append(newPieces, pt.pieces[endIdx+1:]...)
	}

	pt.pieces = newPieces
	pt.size -= length
}

// Bytes assembles and returns all current text.
// Note: for large file rendering in future we'll write ReadAt methods,
// so as not to unload entire buffer into memory.
func (pt *PieceTable) Bytes() ([]byte, error) {
	res := make([]byte, 0, pt.size)
	for _, p := range pt.pieces {
		if p.Buf == Original {
			data, err := pt.orig.Read(p.Start, p.Length)
			if err != nil { return nil, err }
			res = append(res, data...)
		} else {
			res = append(res, pt.add[p.Start:p.Start+p.Length]...)
		}
	}
	return res, nil
}
// AppendRange appends the specified range to the dest slice without new allocations.
func (pt *PieceTable) AppendRange(dest []byte, offset, length int) ([]byte, error) {
	if offset < 0 || length <= 0 {
		return dest, nil
	}
	if offset+length > pt.size {
		length = pt.size - offset
	}

	remaining := length
	startIdx, offInPiece := pt.offsetToPiece(offset)

	for i := startIdx; i < len(pt.pieces) && remaining > 0; i++ {
		p := pt.pieces[i]

		take := p.Length - offInPiece
		if take > remaining {
			take = remaining
		}

		if p.Buf == Original {
			data, err := pt.orig.Read(p.Start+offInPiece, take)
			if err != nil { return dest, err }
			dest = append(dest, data...)
		} else {
			dest = append(dest, pt.add[p.Start+offInPiece : p.Start+offInPiece+take]...)
		}

		remaining -= take
		offInPiece = 0
	}

	return dest, nil
}

// String returns current text as a string (convenient for tests).
// Ignore errors here to keep tests simple.
func (pt *PieceTable) String() string {
	b, _ := pt.Bytes()
	return string(b)
}

// ForEachRange sequentially calls a function for each data fragment.
// This allows processing text without allocating a single large slice.
func (pt *PieceTable) ForEachRange(fn func(data []byte) error) error {
	for _, p := range pt.pieces {
		if p.Buf == Original {
			const chunkSize = 1024 * 1024
			for offset := 0; offset < p.Length; offset += chunkSize {
				take := chunkSize
				if offset+take > p.Length {
					take = p.Length - offset
				}
				data, err := pt.orig.Read(p.Start+offset, take)
				if err != nil { return err }
				if err := fn(data); err != nil { return err }
			}
		} else {
			if err := fn(pt.add[p.Start : p.Start+p.Length]); err != nil { return err }
		}
	}
	return nil
}

// GetRange returns a byte slice for the specified range.
func (pt *PieceTable) GetRange(offset, length int) ([]byte, error) {
	if offset < 0 || length <= 0 || offset+length > pt.size {
		return nil, nil
	}

	res := make([]byte, 0, length)
	remaining := length

	startIdx, offInPiece := pt.offsetToPiece(offset)

	for i := startIdx; i < len(pt.pieces) && remaining > 0; i++ {
		p := pt.pieces[i]

		// Determine how much data we take from this piece
		take := p.Length - offInPiece
		if take > remaining {
			take = remaining
		}

		var buf []byte
		if p.Buf == Original {
			var err error
			buf, err = pt.orig.Read(p.Start+offInPiece, take)
			if err != nil { return nil, err }
		} else {
			buf = pt.add[p.Start+offInPiece : p.Start+offInPiece+take]
		}

		res = append(res, buf...)

		remaining -= take
		offInPiece = 0 // For subsequent pieces, read from start
	}

	return res, nil
}

// UpdateOriginalBuffer safely replaces the original underlying buffer
// without losing the current logical state and additions.
// Used primarily for state recovery after a failed I/O operation.
func (pt *PieceTable) UpdateOriginalBuffer(buf Buffer) {
	pt.orig = buf
}
func NewWithBuffer(buf Buffer) *PieceTable {
	pt := &PieceTable{
		orig: buf,
		size: buf.Size(),
	}
	if buf.Size() > 0 {
		pt.pieces = []Piece{{Buf: Original, Start: 0, Length: buf.Size()}}
	}
	return pt
}
