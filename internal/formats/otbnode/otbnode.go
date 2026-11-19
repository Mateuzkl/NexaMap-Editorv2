// Package otbnode reads and writes the escaped node tree used by items.otb
// and OTBM maps: a 4-byte identifier followed by nodes. A node starts with
// 0xFE and its type byte, holds property bytes and child nodes, and ends
// with 0xFF. Property bytes equal to 0xFD, 0xFE or 0xFF are escaped with 0xFD.
package otbnode

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/nekiro/ots-creator/internal/formats/binio"
)

const (
	nodeStart = 0xFE
	nodeEnd   = 0xFF
	escape    = 0xFD
)

var ErrFormat = errors.New("otbnode: malformed node file")

// Node is one parsed node. Props holds the unescaped property bytes
// (without the type byte).
type Node struct {
	Type     byte
	Props    []byte
	Children []*Node
}

func Parse(data []byte) (root *Node, ident [4]byte, err error) {
	return ParseProgress(data, nil)
}

const progressStep = 1 << 20

func ParseProgress(data []byte, progress func(done, total int)) (root *Node, ident [4]byte, err error) {
	return (&parser{buf: make([]byte, 0, len(data)), progress: progress}).parse(data)
}

// Stream parses a node file without keeping all of it: every node at depth
// (the root is depth 0) is handed to visit with its finished subtree and
// then dropped, so only one such subtree is in memory at a time. stack holds
// the nodes above it, with their properties (their Children lack the
// streamed nodes). Nodes above depth are returned in root as usual.
// Properties and nodes passed to visit are only valid during the call.
func Stream(data []byte, depth int, progress func(done, total int), visit func(stack []*Node, node *Node) error) (root *Node, ident [4]byte, err error) {
	return (&parser{progress: progress, depth: depth, visit: visit, pool: &NodePool{}}).parse(data)
}

func (p *parser) parse(data []byte) (root *Node, ident [4]byte, err error) {
	if len(data) < 6 {
		return nil, ident, fmt.Errorf("%w: file too small", ErrFormat)
	}
	copy(ident[:], data[:4])
	p.data, p.pos, p.nextProgress = data, 4, progressStep
	if p.data[p.pos] != nodeStart {
		return nil, ident, fmt.Errorf("%w: no root node", ErrFormat)
	}
	p.pos++
	root, err = p.node()
	return root, ident, err
}

type parser struct {
	data []byte
	pos  int
	// buf holds every node's unescaped properties back to back; nodes slice
	// into it, so parsing allocates one buffer instead of one per node.
	buf          []byte
	progress     func(done, total int)
	nextProgress int
	// Streaming (see Stream): nodes at depth go to visit, stack is the path.
	depth int
	visit func(stack []*Node, node *Node) error
	stack []*Node
	// nesting is how deep the node being read is, bounded by maxNesting.
	nesting int
	// pool holds reusable Node slabs when parsing streamed/single subtrees.
	pool *NodePool
}

// NodePool reuses Node structs and property buffers across repeated ParseNode
// calls (e.g. one per OTBM tile area on each worker goroutine), avoiding
// millions of small heap allocations.
type NodePool struct {
	slabs   [][]Node
	slabIdx int
	nextIdx int
	buf     []byte
}

const nodeSlabSize = 4096

func (np *NodePool) Reset() {
	np.slabIdx = 0
	np.nextIdx = 0
	np.buf = np.buf[:0]
}

func (np *NodePool) alloc(nodeType byte) *Node {
	if np.slabIdx < len(np.slabs) && np.nextIdx >= len(np.slabs[np.slabIdx]) {
		np.slabIdx++
		np.nextIdx = 0
	}
	if np.slabIdx >= len(np.slabs) {
		np.slabs = append(np.slabs, make([]Node, nodeSlabSize))
	}
	n := &np.slabs[np.slabIdx][np.nextIdx]
	np.nextIdx++
	n.Type = nodeType
	n.Props = nil
	n.Children = n.Children[:0]
	return n
}

// ChildSpan locates one raw node slice [Start:End] (including leading 0xFE and
// trailing 0xFF) inside an OTB/OTBM byte buffer.
type ChildSpan struct {
	Type  byte
	Start int
	End   int
}

// SplitDepth2 parses the root node (depth 0) and its first child (depth 1, e.g.
// OTBM mapData) and returns the raw byte spans of every depth-2 child without
// allocating a node tree for them. Callers can decode those spans in parallel
// using ParseSpan.
func SplitDepth2(data []byte) (root, mapData *Node, ident [4]byte, spans []ChildSpan, err error) {
	if len(data) < 6 {
		return nil, nil, ident, nil, fmt.Errorf("%w: file too small", ErrFormat)
	}
	copy(ident[:], data[:4])
	pos := 4
	if data[pos] != nodeStart {
		return nil, nil, ident, nil, fmt.Errorf("%w: no root node", ErrFormat)
	}
	pos++
	if pos >= len(data) {
		return nil, nil, ident, nil, fmt.Errorf("%w: truncated node", ErrFormat)
	}

	readHeader := func() (*Node, byte, error) {
		n := &Node{Type: data[pos]}
		pos++
		var props []byte
		for pos < len(data) {
			b := data[pos]
			if b == nodeStart || b == nodeEnd {
				n.Props = props
				return n, b, nil
			}
			if b == escape {
				if pos+1 >= len(data) {
					return nil, 0, fmt.Errorf("%w: dangling escape", ErrFormat)
				}
				props = append(props, data[pos+1])
				pos += 2
				continue
			}
			runEnd := pos + 1
			for runEnd < len(data) {
				v := data[runEnd]
				if v == nodeStart || v == nodeEnd || v == escape {
					break
				}
				runEnd++
			}
			props = append(props, data[pos:runEnd]...)
			pos = runEnd
		}
		return nil, 0, fmt.Errorf("%w: missing node end", ErrFormat)
	}

	var term byte
	root, term, err = readHeader()
	if err != nil {
		return nil, nil, ident, nil, err
	}
	if term == nodeEnd {
		return root, nil, ident, nil, nil
	}
	// Enter depth-1 child (nodeMapData).
	pos++
	if pos >= len(data) {
		return nil, nil, ident, nil, fmt.Errorf("%w: truncated node", ErrFormat)
	}
	mapData, term, err = readHeader()
	if err != nil {
		return nil, nil, ident, nil, err
	}
	root.Children = []*Node{mapData}
	if term == nodeEnd {
		return root, mapData, ident, nil, nil
	}

	// Scan depth-2 child spans linearly with zero allocations per node.
	spans = make([]ChildSpan, 0, max(64, len(data)/(32*1024)))
	n := len(data)
	for pos < n {
		b := data[pos]
		if b == nodeEnd {
			return root, mapData, ident, spans, nil
		}
		if b != nodeStart {
			if b == escape {
				pos += 2
			} else {
				pos++
			}
			continue
		}
		start := pos
		if pos+1 >= n {
			return nil, nil, ident, nil, fmt.Errorf("%w: truncated node", ErrFormat)
		}
		childType := data[pos+1]
		pos += 2
		level := 1
		for pos < n && level > 0 {
			c := data[pos]
			if c == escape {
				if pos+1 >= n {
					return nil, nil, ident, nil, fmt.Errorf("%w: dangling escape", ErrFormat)
				}
				pos += 2
				continue
			}
			if c == nodeStart {
				level++
			} else if c == nodeEnd {
				level--
			}
			pos++
		}
		if level != 0 {
			return nil, nil, ident, nil, fmt.Errorf("%w: missing node end", ErrFormat)
		}
		spans = append(spans, ChildSpan{Type: childType, Start: start, End: pos})
	}
	return nil, nil, ident, nil, fmt.Errorf("%w: missing node end", ErrFormat)
}

// ParseSpan decodes a single raw node slice (starting with 0xFE and ending with
// 0xFF) using pool so callers can decode depth-2 subtrees concurrently with
// minimal allocations. The returned tree is valid until pool.Reset().
func ParseSpan(raw []byte, pool *NodePool) (*Node, error) {
	if len(raw) < 3 || raw[0] != nodeStart {
		return nil, fmt.Errorf("%w: invalid span", ErrFormat)
	}
	pool.Reset()
	p := parser{data: raw, pos: 1, buf: pool.buf, pool: pool}
	node, err := p.node()
	pool.buf = p.buf[:0]
	return node, err
}

// maxNesting bounds nodes in nodes: real files stay far below it (a map
// nests a few levels plus containers in containers), and a crafted file
// would otherwise overflow the stack.
const maxNesting = 1024

func (p *parser) node() (*Node, error) {
	if p.pos >= len(p.data) {
		return nil, fmt.Errorf("%w: truncated node", ErrFormat)
	}
	if p.nesting >= maxNesting {
		return nil, fmt.Errorf("%w: nodes nested too deep", ErrFormat)
	}
	p.nesting++
	var node *Node
	if p.pool != nil && len(p.stack) >= p.depth {
		node = p.pool.alloc(p.data[p.pos])
	} else {
		node = &Node{Type: p.data[p.pos]}
	}
	p.pos++
	propsStart := len(p.buf)
	streamChildren := p.visit != nil && len(p.stack)+1 == p.depth
	pushedStack := p.visit != nil
	if pushedStack {
		p.stack = append(p.stack, node)
	}
	childCount := 0
	for p.pos < len(p.data) {
		switch p.data[p.pos] {
		case nodeStart:
			if childCount == 0 {
				// Copied: streamed children reuse the buffer after it.
				node.Props = p.buf[propsStart:len(p.buf):len(p.buf)]
				if streamChildren {
					node.Props = append([]byte(nil), node.Props...)
				}
			}
			childCount++
			p.pos++
			childStart := len(p.buf)
			child, err := p.node()
			if err != nil {
				if pushedStack {
					p.stack = p.stack[:len(p.stack)-1]
				}
				p.nesting--
				return nil, err
			}
			if streamChildren {
				if err := p.visit(p.stack, child); err != nil {
					if pushedStack {
						p.stack = p.stack[:len(p.stack)-1]
					}
					p.nesting--
					return nil, err
				}
				p.buf = p.buf[:childStart]
				if p.pool != nil {
					p.pool.Reset()
				}
			} else {
				node.Children = append(node.Children, child)
			}
			// Properties after children are not part of the format.
			propsStart = len(p.buf)
		case nodeEnd:
			if childCount == 0 {
				node.Props = p.buf[propsStart:len(p.buf):len(p.buf)]
			}
			p.pos++
			if pushedStack {
				p.stack = p.stack[:len(p.stack)-1]
			}
			p.nesting--
			return node, nil
		case escape:
			if p.pos+1 >= len(p.data) {
				if pushedStack {
					p.stack = p.stack[:len(p.stack)-1]
				}
				p.nesting--
				return nil, fmt.Errorf("%w: dangling escape", ErrFormat)
			}
			p.buf = append(p.buf, p.data[p.pos+1])
			p.pos += 2
		default:
			// Copy the run of plain bytes at once.
			runEnd := p.pos + 1
			for runEnd < len(p.data) {
				value := p.data[runEnd]
				if value == nodeStart || value == nodeEnd || value == escape {
					break
				}
				runEnd++
			}
			p.buf = append(p.buf, p.data[p.pos:runEnd]...)
			p.pos = runEnd
			if p.progress != nil && p.pos >= p.nextProgress {
				p.progress(p.pos, len(p.data))
				p.nextProgress = p.pos + progressStep
			}
		}
	}
	if pushedStack {
		p.stack = p.stack[:len(p.stack)-1]
	}
	p.nesting--
	return nil, fmt.Errorf("%w: missing node end", ErrFormat)
}

// Reader reads little-endian values from node properties with a sticky
// error. It wraps binio.Reader and adds the node file string formats;
// read errors are reported as ErrFormat.
type Reader struct {
	props binio.Reader
}

func NewReader(props []byte) *Reader { return &Reader{props: *binio.NewReader(props)} }

func (r *Reader) Err() error {
	err := r.props.Err()
	if err == nil || errors.Is(err, ErrFormat) {
		return err
	}
	return fmt.Errorf("%w: property: %v", ErrFormat, err)
}

func (r *Reader) Remaining() int { return r.props.Remaining() }

func (r *Reader) More() bool { return r.props.Err() == nil && r.props.Remaining() > 0 }

func (r *Reader) U8() uint8 { return r.props.U8() }

func (r *Reader) U16() uint16 { return r.props.U16() }

func (r *Reader) U32() uint32 { return r.props.U32() }

func (r *Reader) U64() uint64 { return r.props.U64() }

func (r *Reader) Bytes(count int) []byte { return r.props.Bytes(count) }

func (r *Reader) Skip(count int) { r.props.Bytes(count) }

func (r *Reader) String() string {
	length := int(r.props.U16())
	return string(r.props.Bytes(length))
}

func (r *Reader) LongString() string {
	length := r.props.U32()
	if length > math.MaxInt32 {
		r.props.Fail(fmt.Errorf("%w: string too long", ErrFormat))
		return ""
	}
	return string(r.props.Bytes(int(length)))
}

type Writer struct {
	out     *bufio.Writer
	err     error
	scratch [8]byte
}

func NewWriter(dest io.Writer, ident [4]byte) *Writer {
	out := bufio.NewWriterSize(dest, 1<<20)
	_, err := out.Write(ident[:])
	return &Writer{out: out, err: err}
}

func (w *Writer) Start(nodeType byte) {
	w.raw(nodeStart)
	w.U8(nodeType)
}

func (w *Writer) End() { w.raw(nodeEnd) }

func (w *Writer) raw(value byte) {
	if w.err == nil {
		w.err = w.out.WriteByte(value)
	}
}

func (w *Writer) U8(value byte) {
	if value == nodeStart || value == nodeEnd || value == escape {
		w.raw(escape)
	}
	w.raw(value)
}

func (w *Writer) Bytes(data []byte) {
	for _, value := range data {
		w.U8(value)
	}
}

func (w *Writer) U16(value uint16) {
	binary.LittleEndian.PutUint16(w.scratch[:], value)
	w.Bytes(w.scratch[:2])
}

func (w *Writer) U32(value uint32) {
	binary.LittleEndian.PutUint32(w.scratch[:], value)
	w.Bytes(w.scratch[:4])
}

func (w *Writer) U64(value uint64) {
	binary.LittleEndian.PutUint64(w.scratch[:], value)
	w.Bytes(w.scratch[:8])
}

// String writes a string with a uint16 length (truncated to 65535 bytes).
func (w *Writer) String(text string) {
	if len(text) > math.MaxUint16 {
		text = text[:math.MaxUint16]
	}
	w.U16(uint16(len(text)))
	w.Bytes([]byte(text))
}

func (w *Writer) LongString(text string) {
	w.U32(uint32(len(text)))
	w.Bytes([]byte(text))
}

func (w *Writer) Flush() error {
	if w.err != nil {
		return w.err
	}
	return w.out.Flush()
}
