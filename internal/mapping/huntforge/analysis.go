package huntforge

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/nekiro/ots-creator/internal/core/otitems"
	"github.com/nekiro/ots-creator/internal/mapping/otmap"
)

// Point2D is an integer 2D grid coordinate.
type Point2D struct{ X, Y int }

// CorridorRun is a straight or diagonal corridor segment learned from the source hunt.
type CorridorRun struct {
	Dir     int       // index in Dirs (E, S, SE, SW)
	Width   int       // measured cross-section width
	Centers []Point2D // cross-section centers in direction order
}

func (r *CorridorRun) Length() int { return len(r.Centers) }

// RoomExit marks where a corridor connects to a room.
type RoomExit struct {
	X, Y   int
	DX, DY int
}

// RoomShape is a wide walkable area (room, junction, or end chamber).
type RoomShape struct {
	Cells          []int
	X1, Y1, X2, Y2 int
	CenterX        int
	CenterY        int
	Exits          []RoomExit
}

func (r *RoomShape) Area() int { return len(r.Cells) }

// Canonical corridor directions and their perpendicular measurement axes.
var (
	Dirs = [4]Point2D{{1, 0}, {0, 1}, {1, 1}, {-1, 1}}
	Perp = [4]Point2D{{0, 1}, {1, 0}, {1, 0}, {1, 0}}
)

// Analysis holds the topology, style metrics, and 5x5/3x3 tile dictionaries learned from a hunt.
type Analysis struct {
	Template *Template
	DB       *otitems.DB
	W, H     int
	Floor    int

	Tiles      []*otmap.Tile
	Walk       []bool
	DistToWall []int
	DistToWalk []int

	WalkArea                int
	CorridorWidth           int
	FillDepth               int
	Separation              int
	LoopsPerRoom            float64
	DiagonalShare           float64
	ObstaclesAsFloor        int
	Runs                    []*CorridorRun
	InRun                   []bool
	CorridorCreatureDensity float64
	RoomCreatureDensity     float64
	CreatureSpacing         int
	Rooms                   []*RoomShape

	Context5         map[int][]int
	Context3         map[int][]int
	CreatureAt       map[int]otmap.Creature
	CommonDecoration map[uint32]bool
	FillStack        []uint32
	FloorStack       []uint32
}

// Analyze inspects a Template on the chosen floor (floor < 0 picks the floor with most walkable tiles).
func Analyze(tpl *Template, db *otitems.DB, floor int) *Analysis {
	w, h := tpl.Width(), tpl.Height()
	if floor < 0 {
		floor = pickMainFloor(tpl, db)
	}
	a := &Analysis{
		Template:         tpl,
		DB:               db,
		W:                w,
		H:                h,
		Floor:            floor,
		Tiles:            make([]*otmap.Tile, w*h),
		Walk:             make([]bool, w*h),
		CreatureSpacing:  3,
		Context5:         map[int][]int{},
		Context3:         map[int][]int{},
		CreatureAt:       map[int]otmap.Creature{},
		CommonDecoration: map[uint32]bool{},
	}
	for _, pt := range tpl.Tiles {
		if pt.Z != floor || pt.X < 0 || pt.X >= w || pt.Y < 0 || pt.Y >= h {
			continue
		}
		idx := pt.Y*w + pt.X
		a.Tiles[idx] = pt.Tile
		a.Walk[idx] = IsWalkable(pt.Tile, db)
	}
	a.ObstaclesAsFloor = a.fillSmallObstacles()
	for _, b := range a.Walk {
		if b {
			a.WalkArea++
		}
	}
	a.DistToWall = Chebyshev(a.Walk, w, h, false)
	a.DistToWalk = Chebyshev(a.Walk, w, h, true)

	for _, pc := range tpl.Creatures {
		if pc.Z == floor && pc.X >= 0 && pc.X < w && pc.Y >= 0 && pc.Y < h {
			a.CreatureAt[pc.Y*w+pc.X] = pc.Creature
		}
	}

	a.measureRuns()
	a.findRooms()
	a.measureFill()
	a.measureSeparation()
	a.measureCreatures()
	a.buildContexts()
	a.learnStacks()
	return a
}

func isGroundType(it *otitems.Type) bool {
	return it != nil && (it.IsGround() || it.Group == otitems.GroupGround)
}

// IsWalkable reports whether a tile is structurally walkable floor.
func IsWalkable(tile *otmap.Tile, db *otitems.DB) bool {
	if tile == nil || tile.Ground.ID == 0 {
		return false
	}
	ground := db.Get32(tile.Ground.ID)
	if !isGroundType(ground) || ground.Flags.Has(otitems.FlagUnpassable) {
		return false
	}
	for _, it := range tile.Items {
		if typ := db.Get32(it.ID); typ != nil && IsStructuralBlock(typ) {
			return false
		}
	}
	return true
}

// IsStructuralBlock identifies walls, mountain rocks, and blocking ground borders.
func IsStructuralBlock(it *otitems.Type) bool {
	return it.Flags.Has(otitems.FlagUnpassable) && (it.TopOrder == 2 || isGroundType(it))
}

// IsStructural reports whether an item defines geometry (ground, border, wall) vs loose decoration.
func (a *Analysis) IsStructural(id uint32) bool {
	it := a.DB.Get32(id)
	return it == nil || isGroundType(it) || it.IsBorder() || IsStructuralBlock(it)
}

func pickMainFloor(tpl *Template, db *otitems.DB) int {
	counts := map[int]int{}
	bestZ, bestN := 0, -1
	for _, pt := range tpl.Tiles {
		if IsWalkable(pt.Tile, db) {
			counts[pt.Z]++
			if counts[pt.Z] > bestN || (counts[pt.Z] == bestN && pt.Z < bestZ) {
				bestN = counts[pt.Z]
				bestZ = pt.Z
			}
		}
	}
	return bestZ
}

func (a *Analysis) In(x, y int) bool {
	return x >= 0 && x < a.W && y >= 0 && y < a.H
}

func (a *Analysis) Key5(x, y int) int { return Key5(a.Walk, a.W, a.H, x, y) }
func (a *Analysis) Key3(x, y int) int { return Key3(a.Walk, a.W, a.H, x, y) }

// Key5 encodes the 5x5 walkability neighborhood around (x,y) into a 25-bit mask.
func Key5(walk []bool, w, h, x, y int) int {
	k := 0
	for dy := -2; dy <= 2; dy++ {
		for dx := -2; dx <= 2; dx++ {
			xx, yy := x+dx, y+dy
			k <<= 1
			if xx >= 0 && xx < w && yy >= 0 && yy < h && walk[yy*w+xx] {
				k |= 1
			}
		}
	}
	return k
}

// Key3 encodes the 3x3 walkability neighborhood around (x,y) into a 9-bit mask.
func Key3(walk []bool, w, h, x, y int) int {
	k := 0
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			xx, yy := x+dx, y+dy
			k <<= 1
			if xx >= 0 && xx < w && yy >= 0 && yy < h && walk[yy*w+xx] {
				k |= 1
			}
		}
	}
	return k
}

// Chebyshev computes an 8-neighbor BFS distance field.
// When invert=false, measures distance to non-walkable cells; when invert=true, to walkable cells.
func Chebyshev(walk []bool, w, h int, invert bool) []int {
	const inf = math.MaxInt32
	d := make([]int, w*h)
	q := make([]int, 0, w*h/4+16)
	for i := range d {
		src := !walk[i]
		if invert {
			src = walk[i]
		}
		if src {
			d[i] = 0
			q = append(q, i)
		} else {
			d[i] = inf
		}
	}
	if !invert {
		seed := func(x, y int) {
			i := y*w + x
			if walk[i] && d[i] > 1 {
				d[i] = 1
				q = append(q, i)
			}
		}
		for x := 0; x < w; x++ {
			if h > 0 {
				seed(x, 0)
				seed(x, h-1)
			}
		}
		for y := 0; y < h; y++ {
			if w > 0 {
				seed(0, y)
				seed(w-1, y)
			}
		}
	}
	for head := 0; head < len(q); head++ {
		i := q[head]
		x, y := i%w, i/w
		nd := d[i] + 1
		for dy := -1; dy <= 1; dy++ {
			yy := y + dy
			if yy < 0 || yy >= h {
				continue
			}
			row := yy * w
			for dx := -1; dx <= 1; dx++ {
				xx := x + dx
				if xx < 0 || xx >= w {
					continue
				}
				j := row + xx
				if d[j] > nd {
					d[j] = nd
					q = append(q, j)
				}
			}
		}
	}
	return d
}

// fillSmallObstacles treats isolated rocks/statues/trees surrounded by walkable floor as floor in topology.
func (a *Analysis) fillSmallObstacles() int {
	const maxCells, maxSpan = 8, 4
	seen := make([]bool, len(a.Walk))
	filled := 0
	var comp []int
	var q []int
	for start := range a.Walk {
		if a.Walk[start] || seen[start] || a.Tiles[start] == nil {
			continue
		}
		comp = comp[:0]
		q = q[:0]
		touchesOutside := false
		q = append(q, start)
		seen[start] = true
		for head := 0; head < len(q); head++ {
			i := q[head]
			comp = append(comp, i)
			x, y := i%a.W, i/a.W
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					xx, yy := x+dx, y+dy
					if !a.In(xx, yy) {
						touchesOutside = true
						continue
					}
					j := yy*a.W + xx
					if a.Tiles[j] == nil {
						touchesOutside = true
						continue
					}
					if !a.Walk[j] && !seen[j] {
						seen[j] = true
						q = append(q, j)
					}
				}
			}
		}
		if touchesOutside || len(comp) > maxCells {
			continue
		}
		x1, x2 := a.W, 0
		y1, y2 := a.H, 0
		groundOk := true
		for _, c := range comp {
			cx, cy := c%a.W, c/a.W
			if cx < x1 {
				x1 = cx
			}
			if cx > x2 {
				x2 = cx
			}
			if cy < y1 {
				y1 = cy
			}
			if cy > y2 {
				y2 = cy
			}
			gid := a.Tiles[c].Ground.ID
			g := a.DB.Get32(gid)
			if !isGroundType(g) || g.Flags.Has(otitems.FlagUnpassable) {
				groundOk = false
			}
		}
		if !groundOk || x2-x1+1 > maxSpan || y2-y1+1 > maxSpan {
			continue
		}
		for _, c := range comp {
			a.Walk[c] = true
		}
		filled += len(comp)
	}
	return filled
}

type span struct{ a, b int }

func (s span) width() int { return s.b - s.a + 1 }

type posSpan struct {
	pos int
	s   span
}

type trackSeg struct {
	shift int
	spans []posSpan
}

func findSpans(walk func(int) bool, n int) []span {
	var list []span
	for i := 0; i < n; {
		if !walk(i) {
			i++
			continue
		}
		start := i
		for i < n && walk(i) {
			i++
		}
		list = append(list, span{start, i - 1})
	}
	return list
}

func trackSpans(spans [][]span, n int) [][]posSpan {
	type pk struct{ p, k int }
	used := map[pk]bool{}
	var tracks [][]posSpan
	for p := 0; p < n; p++ {
		for k := range spans[p] {
			if used[pk{p, k}] {
				continue
			}
			var track []posSpan
			cp, ck := p, k
			for {
				used[pk{cp, ck}] = true
				cur := spans[cp][ck]
				track = append(track, posSpan{cp, cur})
				if cp+1 >= n {
					break
				}
				next := -1
				for j, s := range spans[cp+1] {
					if used[pk{cp + 1, j}] {
						continue
					}
					if absInt(s.a-cur.a) <= 1 && absInt(s.b-cur.b) <= 1 {
						next = j
						break
					}
				}
				if next < 0 {
					break
				}
				cp++
				ck = next
			}
			if len(track) >= 4 {
				tracks = append(tracks, track)
			}
		}
	}
	return tracks
}

func splitTrack(track []posSpan) []trackSeg {
	var out []trackSeg
	for i := 0; i < len(track)-1; {
		shift := track[i+1].s.a - track[i].s.a
		shiftB := track[i+1].s.b - track[i].s.b
		if shift != shiftB {
			i++
			continue
		}
		seg := []posSpan{track[i]}
		j := i + 1
		for j < len(track) && track[j].s.a-track[j-1].s.a == shift && track[j].s.b-track[j-1].s.b == shift {
			seg = append(seg, track[j])
			j++
		}
		out = append(out, trackSeg{shift: shift, spans: seg})
		if j-1 == i {
			i++
		} else {
			i = j - 1
		}
	}
	return out
}

func (a *Analysis) measureRuns() {
	rowSpans := make([][]span, a.H)
	for y := 0; y < a.H; y++ {
		row := y * a.W
		rowSpans[y] = findSpans(func(x int) bool { return a.Walk[row+x] }, a.W)
	}
	colSpans := make([][]span, a.W)
	for x := 0; x < a.W; x++ {
		colSpans[x] = findSpans(func(y int) bool { return a.Walk[y*a.W+x] }, a.H)
	}

	tracksRow := trackSpans(rowSpans, a.H)
	tracksCol := trackSpans(colSpans, a.W)

	var straightWidths []int
	for _, t := range append(append([][]posSpan(nil), tracksRow...), tracksCol...) {
		for _, seg := range splitTrack(t) {
			if seg.shift == 0 && len(seg.spans) >= 4 {
				for _, sp := range seg.spans {
					straightWidths = append(straightWidths, sp.s.width())
				}
			}
		}
	}
	sort.Ints(straightWidths)
	if len(straightWidths) > 0 {
		a.CorridorWidth = straightWidths[int(float64(len(straightWidths))*0.35)]
	} else {
		a.CorridorWidth = 5
	}

	diagLen, totalLen := 0, 0
	addRuns := func(tracks [][]posSpan, rows bool) {
		for _, t := range tracks {
			for _, seg := range splitTrack(t) {
				if len(seg.spans) < 4 {
					continue
				}
				dir := -1
				if rows {
					switch seg.shift {
					case 0:
						dir = 1
					case 1:
						dir = 2
					default:
						dir = 3
					}
				} else if seg.shift == 0 {
					dir = 0
				}
				if dir < 0 {
					continue
				}
				widths := make([]int, len(seg.spans))
				for i, sp := range seg.spans {
					widths[i] = sp.s.width()
				}
				sort.Ints(widths)
				width := widths[len(widths)/2]
				maxW := float64(a.CorridorWidth)*1.4 + 1
				if seg.shift != 0 {
					maxW = float64(a.CorridorWidth)*2.2 + 1
				}
				if float64(width) > maxW || float64(width) < math.Max(2, float64(a.CorridorWidth)*0.6) {
					continue
				}
				run := &CorridorRun{Dir: dir, Width: width, Centers: make([]Point2D, 0, len(seg.spans))}
				for _, sp := range seg.spans {
					c := sp.s.a + width/2
					if rows {
						run.Centers = append(run.Centers, Point2D{c, sp.pos})
					} else {
						run.Centers = append(run.Centers, Point2D{sp.pos, c})
					}
				}
				a.Runs = append(a.Runs, run)
				totalLen += run.Length()
				if dir >= 2 {
					diagLen += run.Length()
				}
			}
		}
	}
	addRuns(tracksRow, true)
	addRuns(tracksCol, false)
	if totalLen > 0 {
		a.DiagonalShare = float64(diagLen) / float64(totalLen)
	}
}

func (a *Analysis) findRooms() {
	inRun := make([]bool, a.W*a.H)
	for _, run := range a.Runs {
		p := Perp[run.Dir]
		for _, center := range run.Centers {
			for side := -1; side <= 1; side += 2 {
				startO := 1
				if side == 1 {
					startO = 0
				}
				for o := startO; ; o++ {
					x := center.X + side*o*p.X
					y := center.Y + side*o*p.Y
					if !a.In(x, y) || !a.Walk[y*a.W+x] {
						break
					}
					inRun[y*a.W+x] = true
				}
			}
		}
	}
	a.InRun = inRun

	comp := make([]int, a.W*a.H)
	for i := range comp {
		comp[i] = -1
	}
	var comps [][]int
	card4 := [4]Point2D{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for start := range comp {
		if !a.Walk[start] || inRun[start] || comp[start] >= 0 {
			continue
		}
		var cells []int
		q := []int{start}
		cid := len(comps)
		comp[start] = cid
		for head := 0; head < len(q); head++ {
			i := q[head]
			cells = append(cells, i)
			x, y := i%a.W, i/a.W
			for _, d := range card4 {
				xx, yy := x+d.X, y+d.Y
				if !a.In(xx, yy) {
					continue
				}
				j := yy*a.W + xx
				if a.Walk[j] && !inRun[j] && comp[j] < 0 {
					comp[j] = cid
					q = append(q, j)
				}
			}
		}
		comps = append(comps, cells)
	}

	rooms := map[int]*RoomShape{}
	minArea := max(6, a.CorridorWidth*a.CorridorWidth/2)
	for c, cells := range comps {
		if len(cells) < minArea {
			continue
		}
		r := &RoomShape{Cells: cells, X1: a.W, Y1: a.H}
		sumX, sumY := 0, 0
		for _, i := range cells {
			x, y := i%a.W, i/a.W
			if x < r.X1 {
				r.X1 = x
			}
			if x > r.X2 {
				r.X2 = x
			}
			if y < r.Y1 {
				r.Y1 = y
			}
			if y > r.Y2 {
				r.Y2 = y
			}
			sumX += x
			sumY += y
		}
		r.CenterX = sumX / len(cells)
		r.CenterY = sumY / len(cells)
		rooms[c] = r
		a.Rooms = append(a.Rooms, r)
	}

	roomAt := func(x, y int) int {
		for rad := 0; rad <= 1; rad++ {
			for yy := y - rad; yy <= y+rad; yy++ {
				for xx := x - rad; xx <= x+rad; xx++ {
					if a.In(xx, yy) && comp[yy*a.W+xx] >= 0 {
						return comp[yy*a.W+xx]
					}
				}
			}
		}
		return -1
	}

	edges := 0
	for _, run := range a.Runs {
		d := Dirs[run.Dir]
		first := run.Centers[0]
		last := run.Centers[len(run.Centers)-1]
		before := roomAt(first.X-d.X, first.Y-d.Y)
		if r1, ok := rooms[before]; ok {
			r1.Exits = append(r1.Exits, RoomExit{first.X, first.Y, d.X, d.Y})
		}
		after := roomAt(last.X+d.X, last.Y+d.Y)
		if r2, ok := rooms[after]; ok {
			r2.Exits = append(r2.Exits, RoomExit{last.X, last.Y, -d.X, -d.Y})
		}
		if before >= 0 && after >= 0 {
			edges++
		}
	}
	loops := max(0, edges-len(a.Rooms)+1)
	if len(a.Rooms) > 0 {
		a.LoopsPerRoom = math.Min(1.0, float64(loops)/float64(len(a.Rooms)))
	} else {
		a.LoopsPerRoom = 0.3
	}
}

func (a *Analysis) measureFill() {
	var depths []int
	for i, tile := range a.Tiles {
		if tile == nil || a.Walk[i] || a.DistToWalk[i] >= math.MaxInt32 {
			continue
		}
		x, y := i%a.W, i/a.W
		edge := false
		for dy := -1; dy <= 1 && !edge; dy++ {
			for dx := -1; dx <= 1; dx++ {
				xx, yy := x+dx, y+dy
				if !a.In(xx, yy) || a.Tiles[yy*a.W+xx] == nil {
					edge = true
					break
				}
			}
		}
		if edge {
			depths = append(depths, a.DistToWalk[i])
		}
	}
	sort.Ints(depths)
	if len(depths) > 0 {
		a.FillDepth = clampInt(depths[len(depths)/4], 2, 12)
	} else {
		a.FillDepth = 4
	}
}

func (a *Analysis) measureSeparation() {
	best := math.MaxInt32
	for y := 0; y < a.H; y++ {
		for x := 0; x < a.W; x++ {
			if !a.Walk[y*a.W+x] {
				continue
			}
			for _, d := range [2]Point2D{{1, 0}, {0, 1}} {
				k := 1
				for a.In(x+k*d.X, y+k*d.Y) && !a.Walk[(y+k*d.Y)*a.W+x+k*d.X] {
					k++
				}
				if k > 1 && a.In(x+k*d.X, y+k*d.Y) && k-1 < best {
					best = k - 1
				}
			}
		}
	}
	if best == math.MaxInt32 {
		a.Separation = 4
	} else {
		a.Separation = max(2, best)
	}
}

func (a *Analysis) measureCreatures() {
	runCells, runCreatures, roomCells, roomCreatures := 0, 0, 0, 0
	for i, w := range a.Walk {
		if !w {
			continue
		}
		_, has := a.CreatureAt[i]
		if a.InRun[i] {
			runCells++
			if has {
				runCreatures++
			}
		} else {
			roomCells++
			if has {
				roomCreatures++
			}
		}
	}
	if runCells > 0 {
		a.CorridorCreatureDensity = float64(runCreatures) / float64(runCells)
	}
	if roomCells > 0 {
		a.RoomCreatureDensity = float64(roomCreatures) / float64(roomCells)
	}
	pos := make([]Point2D, 0, len(a.CreatureAt))
	for i := range a.CreatureAt {
		pos = append(pos, Point2D{i % a.W, i / a.W})
	}
	var nn []int
	for idxA, pa := range pos {
		best := math.MaxInt32
		for idxB, pb := range pos {
			if idxA == idxB {
				continue
			}
			d := max(absInt(pa.X-pb.X), absInt(pa.Y-pb.Y))
			if d < best {
				best = d
			}
		}
		if best < math.MaxInt32 {
			nn = append(nn, best)
		}
	}
	sort.Ints(nn)
	if len(nn) > 0 {
		a.CreatureSpacing = clampInt(nn[len(nn)/2], 2, 8)
	} else {
		a.CreatureSpacing = 3
	}
}

func (a *Analysis) buildContexts() {
	for y := 0; y < a.H; y++ {
		for x := 0; x < a.W; x++ {
			i := y*a.W + x
			if a.Tiles[i] == nil || a.DistToWalk[i] > 3 {
				continue
			}
			k5 := a.Key5(x, y)
			a.Context5[k5] = append(a.Context5[k5], i)
			k3 := a.Key3(x, y)
			a.Context3[k3] = append(a.Context3[k3], i)
		}
	}
}

func tileAllItemIDs(t *otmap.Tile) []uint32 {
	if t == nil {
		return nil
	}
	n := len(t.Items)
	if t.Ground.ID != 0 {
		n++
	}
	out := make([]uint32, 0, n)
	if t.Ground.ID != 0 {
		out = append(out, t.Ground.ID)
	}
	for _, it := range t.Items {
		out = append(out, it.ID)
	}
	return out
}

// StructuralSignature returns a canonical string of the structural item IDs on a tile.
func (a *Analysis) StructuralSignature(t *otmap.Tile) string {
	ids := tileAllItemIDs(t)
	var b strings.Builder
	first := true
	for _, id := range ids {
		if !a.IsStructural(id) {
			continue
		}
		if !first {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Itoa(int(id)))
		first = false
	}
	return b.String()
}

func (a *Analysis) learnStacks() {
	decoCount := map[uint32]int{}
	for _, t := range a.Tiles {
		if t == nil {
			continue
		}
		for _, id := range tileAllItemIDs(t) {
			if !a.IsStructural(id) {
				decoCount[id]++
			}
		}
	}
	for id, n := range decoCount {
		if n >= 3 {
			a.CommonDecoration[id] = true
		}
	}

	mostCommon := func(filter func(int) bool) []uint32 {
		type grp struct {
			n int
			t *otmap.Tile
		}
		groups := map[string]*grp{}
		for i, t := range a.Tiles {
			if t == nil || !filter(i) {
				continue
			}
			sig := a.StructuralSignature(t)
			g := groups[sig]
			if g == nil {
				g = &grp{t: t}
				groups[sig] = g
			}
			g.n++
		}
		var best *grp
		for _, g := range groups {
			if best == nil || g.n > best.n {
				best = g
			}
		}
		if best == nil {
			return nil
		}
		var out []uint32
		for _, id := range tileAllItemIDs(best.t) {
			if a.IsStructural(id) {
				out = append(out, id)
			}
		}
		return out
	}

	a.FillStack = mostCommon(func(i int) bool { return !a.Walk[i] && a.DistToWalk[i] >= 3 && a.DistToWalk[i] < math.MaxInt32 })
	a.FloorStack = mostCommon(func(i int) bool { return a.Walk[i] && a.DistToWall[i] >= 3 })
	if len(a.FloorStack) == 0 {
		a.FloorStack = mostCommon(func(i int) bool { return a.Walk[i] })
	}
}

// Summary converts the analysis into a JSON-serializable summary for the UI.
func (a *Analysis) Summary() AnalysisSummary {
	return AnalysisSummary{
		WalkArea:                a.WalkArea,
		CorridorWidth:           a.CorridorWidth,
		Rooms:                   len(a.Rooms),
		Runs:                    len(a.Runs),
		DiagonalShare:           a.DiagonalShare,
		LoopsPerRoom:            a.LoopsPerRoom,
		Separation:              a.Separation,
		FillDepth:               a.FillDepth,
		ObstaclesAsFloor:        a.ObstaclesAsFloor,
		Context5:                len(a.Context5),
		Context3:                len(a.Context3),
		Creatures:               len(a.CreatureAt),
		CorridorCreatureDensity: a.CorridorCreatureDensity,
		RoomCreatureDensity:     a.RoomCreatureDensity,
		CreatureSpacing:         a.CreatureSpacing,
		Floor:                   a.Floor,
		Width:                   a.W,
		Height:                  a.H,
		Species:                 a.Template.Species(),
		Description:             a.Describe(),
	}
}

func (a *Analysis) Describe() string {
	return fmt.Sprintf(
		"área andável %d tiles, corredor %d de largura, %d salas, %d trechos (%.0f%% diagonal), voltas/sala %.2f, parede mín %d, preenchimento %d, contextos %d×5×5 / %d×3×3, %d criaturas",
		a.WalkArea, a.CorridorWidth, len(a.Rooms), len(a.Runs), a.DiagonalShare*100, a.LoopsPerRoom,
		a.Separation, a.FillDepth, len(a.Context5), len(a.Context3), len(a.CreatureAt),
	)
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
