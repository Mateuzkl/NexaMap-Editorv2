package huntforge

import (
	"fmt"
	"math"
	"math/rand"
	"sort"

	"github.com/nekiro/ots-creator/internal/mapping/otmap"
)

const (
	kindNone     uint8 = 0
	kindIdentity uint8 = 1
	kindRoom     uint8 = 2
	kindCorridor uint8 = 3
)

var (
	dir8      = [8]Point2D{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {-1, -1}, {1, -1}, {-1, 1}}
	sectorDir = [8]Point2D{{1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}, {-1, -1}, {0, -1}, {1, -1}}
)

type corridorCell struct {
	cell int
	src  int
	walk bool
}

type boundaryCacheEntry struct {
	version int
	cells   []int
}

type sigGroup struct {
	sig   string
	cells []int
}

// Generator synthesizes cloned, expanded, random-in-style, or shaped hunts from an Analysis.
type Generator struct {
	a   *Analysis
	o   Options
	rng *rand.Rand

	w, h      int
	walk      []bool
	corr      []int
	kind      []uint8
	dist      []int
	walkCount int
	rooms     int
	corridors int
	loops     int
	target    int
	openings  []RoomExit

	sectorArea   [8]float64
	sectorFails  [8]int
	targetSector int

	cx, cy   float64
	ax, ay   float64
	envScale float64

	walkVersion   int
	boundaryCache map[int]boundaryCacheEntry
	groupCache    map[int][]sigGroup
}

// Generate creates a new hunt from analysis using opts.
func Generate(analysis *Analysis, opts Options) *Result {
	opts = opts.normalized()
	if opts.Mode == ModeClone {
		return cloneTemplate(analysis, opts)
	}
	g := newGenerator(analysis, opts)
	return g.generate()
}

func cloneTemplate(analysis *Analysis, opts Options) *Result {
	src := analysis.Template
	name := opts.Name
	if name == "" {
		name = src.Name + "_clone"
	}
	clone := &Template{
		Name:      name,
		Source:    Region{X1: 0, Y1: 0, X2: max(0, src.Width()-1), Y2: max(0, src.Height()-1), Z1: 0, Z2: max(0, src.Floors()-1)},
		Tiles:     make([]PositionedTile, len(src.Tiles)),
		Creatures: make([]PlacedCreature, 0, len(src.Creatures)),
	}
	for i, pt := range src.Tiles {
		clone.Tiles[i] = PositionedTile{X: pt.X, Y: pt.Y, Z: pt.Z, Tile: pt.Tile.Clone()}
	}
	rng := rand.New(rand.NewSource(opts.Seed))
	for _, pc := range src.Creatures {
		if opts.SpawnDensity >= 1.0 || rng.Float64() < opts.SpawnDensity {
			clone.Creatures = append(clone.Creatures, pc)
		}
	}
	return &Result{
		Hunt:      clone,
		WalkArea:  analysis.WalkArea,
		Creatures: len(clone.Creatures),
		Report:    fmt.Sprintf("Clonar: %d tiles andáveis, %d criaturas", analysis.WalkArea, len(clone.Creatures)),
		Species:   clone.Species(),
		Width:     clone.Width(),
		Height:    clone.Height(),
	}
}

func newGenerator(analysis *Analysis, opts Options) *Generator {
	return &Generator{
		a:             analysis,
		o:             opts,
		rng:           rand.New(rand.NewSource(opts.Seed)),
		targetSector:  -1,
		ax:            1e9,
		ay:            1e9,
		envScale:      1.0,
		boundaryCache: map[int]boundaryCacheEntry{},
		groupCache:    map[int][]sigGroup{},
	}
}

func (g *Generator) clearDist() int { return g.a.Separation + 1 }

func (g *Generator) generate() *Result {
	if g.o.Mode == ModeMix {
		return g.generateShaped()
	}
	scale := math.Sqrt(math.Max(1.0, g.o.AreaFactor))
	pad := g.a.FillDepth + 4
	g.w = int(float64(g.a.W)*scale*1.8) + 2*pad + 60
	g.h = int(float64(g.a.H)*scale*1.8) + 2*pad + 60
	g.initCanvas(g.w, g.h)

	target := int(float64(g.a.WalkArea) * g.o.AreaFactor)
	g.target = target
	origin := Point2D{(g.w - g.a.W) / 2, (g.h - g.a.H) / 2}

	if g.o.Mode == ModeExpand {
		for y := 0; y < g.a.H; y++ {
			for x := 0; x < g.a.W; x++ {
				s := y*g.a.W + x
				d := (origin.Y+y)*g.w + origin.X + x
				g.corr[d] = s
				g.kind[d] = kindIdentity
				if g.a.Walk[s] {
					g.walk[d] = true
					g.walkCount++
				}
			}
		}
	} else {
		var start *RoomShape
		var multi []*RoomShape
		for _, r := range g.a.Rooms {
			if len(r.Exits) >= 2 {
				multi = append(multi, r)
			}
		}
		if len(multi) > 0 {
			start = multi[g.rng.Intn(len(multi))]
		} else if len(g.a.Rooms) > 0 {
			start = g.a.Rooms[g.rng.Intn(len(g.a.Rooms))]
		}
		if start != nil {
			g.placeRoom(start, g.w/2-start.CenterX, g.h/2-start.CenterY, nil)
		} else {
			g.stampSquare(g.w/2, g.h/2, g.a.CorridorWidth)
			for i := 0; i < 4; i++ {
				d := dir8[i]
				g.openings = append(g.openings, RoomExit{g.w/2 + d.X*g.a.CorridorWidth, g.h/2 + d.Y*g.a.CorridorWidth, d.X, d.Y})
			}
		}
	}

	g.recomputeDistance()
	g.setupEnvelope()
	attempts, failsInRow := 0, 0
	g.target = int(float64(target) * 0.93)
	for (g.walkCount < g.target || (g.anyLagging() && float64(g.walkCount) < float64(target)*1.1)) && attempts < 8000 && failsInRow < 2000 {
		attempts++
		if g.tryGrow() {
			failsInRow = 0
			g.recomputeDistance()
		} else {
			failsInRow++
			if failsInRow%250 == 0 && g.envScale < 1.6 {
				g.envScale *= 1.06
			}
		}
	}

	loop := g.o.LoopChance
	if loop < 0 {
		loop = math.Max(0.2, math.Min(0.8, g.a.LoopsPerRoom))
	}
	wantLoops := int(math.Round(float64(g.rooms) * loop))
	if g.o.Mode == ModeExpand {
		wantLoops++
	}
	for k := 0; k < 300 && g.loops < wantLoops; k++ {
		if g.tryLoop() {
			g.recomputeDistance()
		}
	}

	misses := g.repairMask()
	res := g.skin(origin)
	res.RoomsAdded = g.rooms
	res.CorridorsAdded = g.corridors
	res.ContextMisses = misses
	ratio := float64(res.WalkArea) / float64(max(1, g.a.WalkArea)) * 100
	warn := ""
	if len(g.a.Runs) == 0 || len(g.a.Rooms) == 0 {
		warn = " — AVISO: não achei corredores nesta hunt (caverna aberta / campo); resultados podem ficar simples."
	}
	res.Report = fmt.Sprintf(
		"%s: área %d → %d tiles andáveis (%.0f%%), %d salas e %d corredores novos, %d criaturas, %d vizinhanças sem exemplo, seed %d%s",
		g.o.Mode, g.a.WalkArea, res.WalkArea, ratio, g.rooms, g.corridors, res.Creatures, misses, g.o.Seed, warn,
	)
	return res
}

func (g *Generator) initCanvas(w, h int) {
	g.w, g.h = w, h
	n := w * h
	g.walk = make([]bool, n)
	g.corr = make([]int, n)
	for i := range g.corr {
		g.corr[i] = -1
	}
	g.kind = make([]uint8, n)
}

func (g *Generator) sector(x, y int) int {
	ang := math.Atan2(float64(y)-g.cy, float64(x)-g.cx)
	s := int(math.Round(ang / (math.Pi / 4)))
	return ((s % 8) + 8) % 8
}

func (g *Generator) lagging(s int) bool {
	if s < 0 {
		return false
	}
	sum := 0.0
	for _, v := range g.sectorArea {
		sum += v
	}
	mean := sum / 8.0
	return mean > 0 && g.sectorArea[s] < mean*0.45
}

func (g *Generator) anyLagging() bool {
	if !g.o.Balanced {
		return false
	}
	for k := 0; k < 8; k++ {
		if g.lagging(k) && g.sectorFails[k] < 60 {
			return true
		}
	}
	return false
}

func (g *Generator) setupEnvelope() {
	x1, y1 := math.MaxInt32, math.MaxInt32
	x2, y2 := -1, -1
	for i, w := range g.walk {
		if !w {
			continue
		}
		x, y := i%g.w, i/g.w
		if x < x1 {
			x1 = x
		}
		if x > x2 {
			x2 = x
		}
		if y < y1 {
			y1 = y
		}
		if y > y2 {
			y2 = y
		}
	}
	if x1 == math.MaxInt32 {
		g.cx, g.cy = float64(g.w)/2.0, float64(g.h)/2.0
		return
	}
	g.cx = float64(x1+x2) / 2.0
	g.cy = float64(y1+y2) / 2.0
	half := math.Max(float64(max(x2-x1, y2-y1))/2.0, float64(g.a.CorridorWidth*3))
	if g.o.Mode == ModeRandom {
		half = math.Max(float64(max(g.a.W, g.a.H))/2.0-float64(g.a.FillDepth), half)
	}
	r := half*math.Sqrt(math.Max(1.0, g.o.AreaFactor))*1.08 + float64(g.a.CorridorWidth)
	g.ax, g.ay = r, r
}

func (g *Generator) inEnvelope(x, y int) bool {
	if !g.o.Balanced {
		return true
	}
	dx := (float64(x) - g.cx) / (g.ax * g.envScale)
	dy := (float64(y) - g.cy) / (g.ay * g.envScale)
	return dx*dx+dy*dy <= 1.0
}

func (g *Generator) countSector(cell int) {
	g.sectorArea[g.sector(cell%g.w, cell/g.w)]++
}

func (g *Generator) pickSector() int {
	penalty := float64(max(20, g.a.CorridorWidth*g.a.CorridorWidth))
	best := 0
	bestScore := math.MaxFloat64
	for k := 0; k < 8; k++ {
		score := g.sectorArea[k] + float64(g.sectorFails[k])*penalty + g.rng.Float64()*penalty
		if score < bestScore {
			bestScore = score
			best = k
		}
	}
	return best
}

func pointsTo(dx, dy, sector int) bool {
	sd := sectorDir[sector]
	dot := float64(dx*sd.X + dy*sd.Y)
	return dot >= 0.7*math.Sqrt(float64(dx*dx+dy*dy))*math.Sqrt(float64(sd.X*sd.X+sd.Y*sd.Y))-1e-9
}

func (g *Generator) tryGrow() bool {
	ok := g.tryGrowCore()
	if g.targetSector >= 0 {
		if ok {
			g.sectorFails[g.targetSector] = 0
		} else {
			g.sectorFails[g.targetSector]++
		}
	}
	return ok
}

func (g *Generator) tryGrowCore() bool {
	var op RoomExit
	opIndex := -1
	sector := -1
	if g.o.Balanced {
		sector = g.pickSector()
	}
	g.targetSector = sector

	var inSector []int
	for k, o := range g.openings {
		if sector < 0 || (g.sector(o.X, o.Y) == sector && pointsTo(o.DX, o.DY, sector)) {
			inSector = append(inSector, k)
		}
	}
	fromWall := len(inSector) == 0 || (g.o.Mode == ModeExpand && g.rng.Float64() < 0.35)
	if fromWall {
		var ok bool
		op, ok = g.findWallOpening(sector, 1.0/3.0)
		if !ok {
			if len(inSector) == 0 {
				return false
			}
			opIndex = inSector[g.rng.Intn(len(inSector))]
			op = g.openings[opIndex]
		}
	} else {
		opIndex = inSector[g.rng.Intn(len(inSector))]
		op = g.openings[opIndex]
	}

	cdir, _ := canonicalDir(op.DX, op.DY)
	sumLen, sameCount := 0, 0
	for _, r := range g.a.Runs {
		if r.Dir == cdir {
			sumLen += r.Length()
			sameCount++
		}
	}
	if sameCount == 0 {
		if opIndex >= 0 {
			g.removeOpeningAt(opIndex)
		}
		return false
	}
	typical := sumLen / sameCount
	length := g.clearDist() + 2 + g.rng.Intn(max(3, typical*3/2))

	var corridor []corridorCell
	ex, ey := g.drawCorridor(op.X, op.Y, op.DX, op.DY, length, &corridor)

	type roomOpt struct {
		room *RoomShape
		exit RoomExit
	}
	var options []roomOpt
	for _, r := range g.a.Rooms {
		for _, e := range r.Exits {
			if e.DX == -op.DX && e.DY == -op.DY {
				options = append(options, roomOpt{r, e})
			}
		}
	}
	if len(options) == 0 {
		if opIndex >= 0 {
			g.removeOpeningAt(opIndex)
		}
		return false
	}

	corrWalk := 0
	for _, c := range corridor {
		if c.walk {
			corrWalk++
		}
	}
	remaining := max(0, g.target-g.walkCount-corrWalk/2)
	limit := float64(remaining)*1.25 + float64(g.a.CorridorWidth*8)
	var fitting []roomOpt
	for _, opt := range options {
		if float64(opt.room.Area()+corrWalk/2) <= limit {
			fitting = append(fitting, opt)
		}
	}
	if len(fitting) == 0 && g.lagging(sector) {
		minArea := math.MaxInt32
		for _, opt := range options {
			if opt.room.Area() < minArea {
				minArea = opt.room.Area()
			}
		}
		for _, opt := range options {
			if opt.room.Area() == minArea {
				fitting = append(fitting, opt)
			}
		}
	}
	if len(fitting) == 0 {
		return false
	}
	chosen := fitting[g.rng.Intn(len(fitting))]
	ox, oy := ex-chosen.exit.X, ey-chosen.exit.Y

	roomSet := make(map[int]bool, len(chosen.room.Cells))
	for _, s := range chosen.room.Cells {
		x, y := s%g.a.W+ox, s/g.a.W+oy
		if !g.inside(x, y, g.a.FillDepth+2) {
			return false
		}
		d := y*g.w + x
		if g.dist[d] < g.clearDist()+1 {
			return false
		}
		if !g.inEnvelope(x, y) {
			return false
		}
		roomSet[d] = true
	}
	for _, c := range corridor {
		if c.walk && !g.inEnvelope(c.cell%g.w, c.cell/g.w) {
			return false
		}
	}
	if !g.corridorClear(corridor, roomSet, op.X, op.Y, ex, ey, true, -1) {
		return false
	}

	g.commit(corridor, kindCorridor)
	g.placeRoom(chosen.room, ox, oy, &chosen.exit)
	if opIndex >= 0 {
		g.removeOpeningAt(opIndex)
	}
	g.rooms++
	g.corridors++
	return true
}

func (g *Generator) findWallOpening(sector int, tolerance float64) (RoomExit, bool) {
	w := g.a.CorridorWidth
	order := [8]int{0, 1, 2, 3, 4, 5, 6, 7}
	for attempt := 0; attempt < 40; attempt++ {
		a := g.randomBoundaryCell(sector)
		if a < 0 {
			return RoomExit{}, false
		}
		ax, ay := a%g.w, a/g.w
		g.rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
		for _, idx := range order {
			d := dir8[idx]
			if sector >= 0 && !pointsTo(d.X, d.Y, sector) {
				continue
			}
			cdir, _ := canonicalDir(d.X, d.Y)
			if !g.hasRunDir(cdir) {
				continue
			}
			wall := true
			for k := 1; k <= g.clearDist()+1 && wall; k++ {
				x, y := ax+k*d.X, ay+k*d.Y
				if !g.inside(x, y, g.a.FillDepth+2) || g.walk[y*g.w+x] {
					wall = false
				}
			}
			if !wall {
				continue
			}
			p := Perp[cdir]
			lo, hi := 0, 0
			for g.isWalk(ax-(lo+1)*p.X, ay-(lo+1)*p.Y) {
				lo++
			}
			for g.isWalk(ax+(hi+1)*p.X, ay+(hi+1)*p.Y) {
				hi++
			}
			span := lo + hi + 1
			if span < w {
				continue
			}
			var start int
			if span <= w*3 {
				start = -lo + (span-w)/2
			} else {
				start = clampInt(-w/2, -lo, hi-w+1)
			}
			bad := 0
			for o := start; o < start+w; o++ {
				x, y := ax+o*p.X, ay+o*p.Y
				if !g.isWalk(x, y) || g.isWalk(x+d.X, y+d.Y) {
					bad++
				}
			}
			if bad > int(float64(w)*tolerance) {
				continue
			}
			c := start + w/2
			return RoomExit{ax + c*p.X, ay + c*p.Y, d.X, d.Y}, true
		}
	}
	return RoomExit{}, false
}

func (g *Generator) isWalk(x, y int) bool { return g.inside(x, y, 0) && g.walk[y*g.w+x] }

func (g *Generator) tryLoop() bool {
	if len(g.openings) == 0 {
		return false
	}
	w := g.a.CorridorWidth
	ai := g.rng.Intn(len(g.openings))
	a := g.openings[ai]
	bi, best := -1, math.MaxInt32
	for k, b := range g.openings {
		if k == ai {
			continue
		}
		d := max(absInt(b.X-a.X), absInt(b.Y-a.Y))
		dot := (b.X-a.X)*a.DX + (b.Y-a.Y)*a.DY
		if d < w*2 || d > w*14 || dot <= 0 {
			continue
		}
		if d < best {
			best = d
			bi = k
		}
	}
	if bi < 0 {
		return false
	}
	return g.connectOpenings(a, g.openings[bi])
}

func (g *Generator) connectOpenings(a, b RoomExit) bool {
	w := g.a.CorridorWidth
	ddx, ddy := b.X-a.X, b.Y-a.Y
	diagSteps := min(absInt(ddx), absInt(ddy))
	straight := max(absInt(ddx), absInt(ddy)) - diagSteps
	sx, sy := signInt(ddx), signInt(ddy)
	straightDir := Point2D{0, sy}
	if absInt(ddx) > absInt(ddy) {
		straightDir = Point2D{sx, 0}
	}
	if diagSteps > 0 && !g.hasRunFor(sx, sy) {
		return false
	}
	if straight > 0 && !g.hasRunFor(straightDir.X, straightDir.Y) {
		return false
	}
	var corridor []corridorCell
	px, py := a.X, a.Y
	if a.DX == sx && a.DY == sy {
		px, py = g.drawCorridor(px, py, sx, sy, diagSteps, &corridor)
		g.addJoint(px, py, &corridor)
		px, py = g.drawCorridor(px, py, straightDir.X, straightDir.Y, straight, &corridor)
	} else {
		px, py = g.drawCorridor(px, py, straightDir.X, straightDir.Y, straight, &corridor)
		g.addJoint(px, py, &corridor)
		px, py = g.drawCorridor(px, py, sx, sy, diagSteps, &corridor)
	}
	g.addJoint(px, py, &corridor)
	if max(absInt(px-b.X), absInt(py-b.Y)) > w/2 {
		return false
	}
	if !g.corridorClear(corridor, nil, a.X, a.Y, b.X, b.Y, false, -1) {
		return false
	}
	g.commit(corridor, kindCorridor)
	g.removeOpening(a)
	g.removeOpening(b)
	g.corridors++
	g.loops++
	return true
}

func (g *Generator) corridorClear(corridor []corridorCell, own map[int]bool, ax, ay, bx, by int, ignoreStartOnly bool, ignoreRadius int) bool {
	if ignoreRadius < 0 {
		ignoreRadius = g.a.CorridorWidth + g.clearDist() + 1
	}
	for _, c := range corridor {
		if !c.walk {
			continue
		}
		x, y := c.cell%g.w, c.cell/g.w
		if !g.inside(x, y, g.a.FillDepth+2) {
			return false
		}
		if own[c.cell] {
			continue
		}
		da := max(absInt(x-ax), absInt(y-ay))
		db := max(absInt(x-bx), absInt(y-by))
		if da <= ignoreRadius || (!ignoreStartOnly && db <= ignoreRadius) {
			continue
		}
		if g.dist[c.cell] < g.clearDist()+1 {
			return false
		}
	}
	return true
}

func (g *Generator) randomBoundaryCell(sector int) int {
	c, ok := g.boundaryCache[sector]
	if !ok || c.version != g.walkVersion {
		var cells []int
		for i, w := range g.walk {
			if !w {
				continue
			}
			x, y := i%g.w, i/g.w
			if sector >= 0 && g.sector(x, y) != sector {
				continue
			}
			if !g.isWalk(x+1, y) || !g.isWalk(x-1, y) || !g.isWalk(x, y+1) || !g.isWalk(x, y-1) {
				cells = append(cells, i)
			}
		}
		c = boundaryCacheEntry{version: g.walkVersion, cells: cells}
		g.boundaryCache[sector] = c
	}
	if len(c.cells) == 0 {
		return -1
	}
	return c.cells[g.rng.Intn(len(c.cells))]
}

func (g *Generator) inside(x, y, margin int) bool {
	return x >= margin && y >= margin && x < g.w-margin && y < g.h-margin
}

func canonicalDir(sx, sy int) (int, int) {
	sign := 1
	if sy < 0 || (sy == 0 && sx < 0) {
		sx, sy = -sx, -sy
		sign = -1
	}
	for d := 0; d < 4; d++ {
		if Dirs[d].X == sx && Dirs[d].Y == sy {
			return d, sign
		}
	}
	return 0, sign
}

func (g *Generator) hasRunDir(dir int) bool {
	for _, r := range g.a.Runs {
		if r.Dir == dir {
			return true
		}
	}
	return false
}

func (g *Generator) hasRunFor(sx, sy int) bool {
	dir, _ := canonicalDir(sx, sy)
	return g.hasRunDir(dir)
}

func (g *Generator) drawCorridor(x, y, sx, sy, steps int, out *[]corridorCell) (int, int) {
	if steps <= 0 {
		return x, y
	}
	dir, sign := canonicalDir(sx, sy)
	var runs []*CorridorRun
	for _, r := range g.a.Runs {
		if r.Dir == dir {
			runs = append(runs, r)
		}
	}
	if len(runs) == 0 {
		return x, y
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].Length() > runs[j].Length() })
	p := Perp[dir]
	cd := Dirs[dir]
	reach := g.a.CorridorWidth/2 + g.a.Separation + 3

	run := runs[g.rng.Intn(min(3, len(runs)))]
	n := run.Length()
	j := g.rng.Intn(n)
	cx, cy := x, y
	for k := 0; k < steps; k++ {
		sc := run.Centers[j]
		for side := -1; side <= 1; side += 2 {
			open := true
			startO := 1
			if side == 1 {
				startO = 0
			}
			for o := startO; o <= reach; o++ {
				so := side * o
				xx, yy := cx+so*p.X, cy+so*p.Y
				if !g.inside(xx, yy, 0) {
					break
				}
				srx, sry := sc.X+so*p.X, sc.Y+so*p.Y
				src := -1
				if g.a.In(srx, sry) {
					src = sry*g.a.W + srx
				}
				walk := open && src >= 0 && g.a.Walk[src]
				if !walk {
					open = false
				}
				*out = append(*out, corridorCell{yy*g.w + xx, src, walk})
			}
		}
		if k == steps-1 {
			break
		}
		nj := j + sign
		var ddx, ddy int
		if nj >= 0 && nj < n {
			ddx = run.Centers[nj].X - sc.X
			ddy = run.Centers[nj].Y - sc.Y
		} else {
			ddx = cd.X * sign
			ddy = cd.Y * sign
			nj = modInt(nj, n)
		}
		cx += ddx
		cy += ddy
		j = nj
	}
	return cx, cy
}

func (g *Generator) addJoint(x, y int, out *[]corridorCell) {
	r := g.a.CorridorWidth / 2
	for yy := y - r; yy <= y+r; yy++ {
		for xx := x - r; xx <= x+r; xx++ {
			if g.inside(xx, yy, 0) {
				*out = append(*out, corridorCell{yy*g.w + xx, -1, true})
			}
		}
	}
}

func (g *Generator) commit(cells []corridorCell, kind uint8) {
	for _, c := range cells {
		if c.walk && !g.walk[c.cell] {
			g.walk[c.cell] = true
			g.walkCount++
			g.countSector(c.cell)
		}
		if g.kind[c.cell] == kindNone && c.src >= 0 {
			g.corr[c.cell] = c.src
			g.kind[c.cell] = kind
		}
	}
}

func (g *Generator) placeRoom(room *RoomShape, ox, oy int, used *RoomExit) {
	margin := g.a.Separation + 2
	cells := make(map[int]bool, len(room.Cells))
	near := make(map[int]bool, len(room.Cells)*4)
	for _, c := range room.Cells {
		cells[c] = true
		x0, y0 := c%g.a.W, c/g.a.W
		for y := y0 - margin; y <= y0+margin; y++ {
			for x := x0 - margin; x <= x0+margin; x++ {
				if g.a.In(x, y) {
					near[y*g.a.W+x] = true
				}
			}
		}
	}
	for s := range near {
		x, y := s%g.a.W+ox, s/g.a.W+oy
		if !g.inside(x, y, 0) {
			continue
		}
		d := y*g.w + x
		if cells[s] && !g.walk[d] {
			g.walk[d] = true
			g.walkCount++
			g.countSector(d)
		}
		if g.kind[d] == kindNone {
			g.corr[d] = s
			g.kind[d] = kindRoom
		}
	}
	for _, e := range room.Exits {
		if used != nil && *used == e {
			continue
		}
		g.openings = append(g.openings, RoomExit{e.X + ox, e.Y + oy, e.DX, e.DY})
	}
}

func (g *Generator) stampSquare(cx, cy, r int) {
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			if !g.inside(x, y, 0) {
				continue
			}
			d := y*g.w + x
			if !g.walk[d] {
				g.walk[d] = true
				g.walkCount++
			}
		}
	}
}

func (g *Generator) recomputeDistance() {
	g.dist = Chebyshev(g.walk, g.w, g.h, true)
	g.walkVersion++
}

func (g *Generator) removeOpeningAt(idx int) {
	g.openings = append(g.openings[:idx], g.openings[idx+1:]...)
}

func (g *Generator) removeOpening(op RoomExit) {
	for i, o := range g.openings {
		if o == op {
			g.removeOpeningAt(i)
			return
		}
	}
}

func (g *Generator) hasOpening(op RoomExit) bool {
	for _, o := range g.openings {
		if o == op {
			return true
		}
	}
	return false
}

// repairMask flips generated boundary cells whose 3x3 pattern never occurs in the source hunt.
func (g *Generator) repairMask() int {
	for pass := 0; pass < 4; pass++ {
		fixedCount := 0
		for y := 1; y < g.h-1; y++ {
			for x := 1; x < g.w-1; x++ {
				i := y*g.w + x
				if g.kind[i] == kindIdentity || !g.nearWalk(x, y) {
					continue
				}
				if _, ok := g.a.Context3[Key3(g.walk, g.w, g.h, x, y)]; ok {
					continue
				}
				before := g.unseen3(x, y)
				g.walk[i] = !g.walk[i]
				after := g.unseen3(x, y)
				if after < before {
					fixedCount++
					if g.walk[i] {
						g.walkCount++
					} else {
						g.walkCount--
					}
				} else {
					g.walk[i] = !g.walk[i]
				}
			}
		}
		if fixedCount == 0 {
			break
		}
	}
	misses := 0
	for y := 1; y < g.h-1; y++ {
		for x := 1; x < g.w-1; x++ {
			if g.nearWalk(x, y) {
				if _, ok := g.a.Context3[Key3(g.walk, g.w, g.h, x, y)]; !ok {
					misses++
				}
			}
		}
	}
	return misses
}

func (g *Generator) nearWalk(x, y int) bool {
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if g.inside(x+dx, y+dy, 0) && g.walk[(y+dy)*g.w+x+dx] {
				return true
			}
		}
	}
	return false
}

func (g *Generator) unseen3(x, y int) int {
	n := 0
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			xx, yy := x+dx, y+dy
			if !g.inside(xx, yy, 1) || !g.nearWalk(xx, yy) {
				continue
			}
			if _, ok := g.a.Context3[Key3(g.walk, g.w, g.h, xx, yy)]; !ok {
				n++
			}
		}
	}
	return n
}

func (g *Generator) skin(origin Point2D) *Result {
	distWalk := Chebyshev(g.walk, g.w, g.h, true)
	outside := g.outsideVoid(distWalk)
	tiles := map[int]*otmap.Tile{}
	var creatures []PlacedCreature
	var corridorCells []int
	walkArea := 0

	for y := 0; y < g.h; y++ {
		for x := 0; x < g.w; x++ {
			i := y*g.w + x
			walk := g.walk[i]
			if walk {
				walkArea++
			}
			src := g.corr[i]
			var srcTile *otmap.Tile
			if src >= 0 {
				srcTile = g.a.Tiles[src]
			}
			key5 := Key5(g.walk, g.w, g.h, x, y)
			sameContext := srcTile != nil && key5 == g.a.Key5(src%g.a.W, src/g.a.W)

			var tile *otmap.Tile
			if g.kind[i] == kindIdentity && srcTile != nil && (sameContext || distWalk[i] > 2) {
				tile = srcTile.Clone()
			} else if outside[i] {
				continue
			} else if sameContext {
				tile = g.copyStack(srcTile, true)
			} else {
				tile = g.fromDictionary(key5, x, y, walk)
			}
			if tile == nil {
				continue
			}
			tiles[i] = tile

			if walk && src >= 0 && (g.kind[i] == kindIdentity || g.kind[i] == kindRoom) {
				if cr, ok := g.a.CreatureAt[src]; ok && (g.kind[i] == kindIdentity || g.a.Key3(src%g.a.W, src/g.a.W) == Key3(g.walk, g.w, g.h, x, y)) {
					if g.kind[i] == kindIdentity || g.rng.Float64() < g.o.SpawnDensity {
						creatures = append(creatures, PlacedCreature{X: x, Y: y, Z: 0, Creature: cr})
					}
				}
			} else if walk && g.kind[i] != kindIdentity && g.kind[i] != kindRoom {
				corridorCells = append(corridorCells, i)
			}
		}
	}

	g.spawnCorridors(&creatures, corridorCells)
	if g.o.SpawnDensity > 1.0 {
		g.addExtraCreatures(&creatures, tiles)
	}
	creatures = thinCreatures(creatures)

	x1, y1 := math.MaxInt32, math.MaxInt32
	x2, y2 := 0, 0
	for i := range tiles {
		x, y := i%g.w, i/g.w
		if x < x1 {
			x1 = x
		}
		if y < y1 {
			y1 = y
		}
		if x > x2 {
			x2 = x
		}
		if y > y2 {
			y2 = y
		}
	}
	if len(tiles) == 0 {
		x1, y1 = 0, 0
	}
	name := g.o.Name
	if name == "" {
		name = g.a.Template.Name + "_" + string(g.o.Mode)
	}
	hunt := &Template{
		Name:      name,
		Source:    Region{X1: 0, Y1: 0, X2: max(0, x2-x1), Y2: max(0, y2-y1), Z1: 0, Z2: 0},
		Tiles:     make([]PositionedTile, 0, len(tiles)),
		Creatures: make([]PlacedCreature, 0, len(creatures)),
	}
	for i, tile := range tiles {
		hunt.Tiles = append(hunt.Tiles, PositionedTile{
			X:    i%g.w - x1,
			Y:    i/g.w - y1,
			Z:    0,
			Tile: tile,
		})
	}
	for _, pc := range creatures {
		hunt.Creatures = append(hunt.Creatures, PlacedCreature{
			X:        pc.X - x1,
			Y:        pc.Y - y1,
			Z:        0,
			Creature: pc.Creature,
		})
	}

	return &Result{
		Hunt:           hunt,
		WalkArea:       walkArea,
		Creatures:      len(hunt.Creatures),
		OriginalOffset: [3]int{origin.X - x1, origin.Y - y1, 0},
		Species:        hunt.Species(),
		Width:          hunt.Width(),
		Height:         hunt.Height(),
	}
}

func (g *Generator) outsideVoid(distWalk []int) []bool {
	n := len(g.walk)
	cand := make([]bool, n)
	for i := 0; i < n; i++ {
		identityVoid := g.kind[i] == kindIdentity && (g.corr[i] < 0 || g.a.Tiles[g.corr[i]] == nil)
		if identityVoid {
			cand[i] = distWalk[i] > 2
		} else {
			cand[i] = g.kind[i] != kindIdentity && distWalk[i] > g.a.FillDepth
		}
	}
	eroded := make([]bool, n)
	for y := 1; y < g.h-1; y++ {
		for x := 1; x < g.w-1; x++ {
			all := true
			for dy := -1; dy <= 1 && all; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if !cand[(y+dy)*g.w+x+dx] {
						all = false
						break
					}
				}
			}
			eroded[y*g.w+x] = all
		}
	}
	opened := make([]bool, n)
	for y := 0; y < g.h; y++ {
		for x := 0; x < g.w; x++ {
			i := y*g.w + x
			if !cand[i] {
				continue
			}
			if x == 0 || y == 0 || x == g.w-1 || y == g.h-1 {
				opened[i] = true
				continue
			}
			for dy := -1; dy <= 1 && !opened[i]; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if eroded[(y+dy)*g.w+x+dx] {
						opened[i] = true
						break
					}
				}
			}
		}
	}
	outside := make([]bool, n)
	q := make([]int, 0, g.w*2+g.h*2)
	seed := func(i int) {
		if opened[i] && !outside[i] {
			outside[i] = true
			q = append(q, i)
		}
	}
	for x := 0; x < g.w; x++ {
		seed(x)
		seed((g.h-1)*g.w + x)
	}
	for y := 0; y < g.h; y++ {
		seed(y * g.w)
		seed(y*g.w + g.w - 1)
	}
	for head := 0; head < len(q); head++ {
		i := q[head]
		x, y := i%g.w, i/g.w
		if x > 0 {
			seed(i - 1)
		}
		if x < g.w-1 {
			seed(i + 1)
		}
		if y > 0 {
			seed(i - g.w)
		}
		if y < g.h-1 {
			seed(i + g.w)
		}
	}
	return outside
}

// copyStack duplicates structural & decoration items while stripping unique/action/text/teleport metadata.
func (g *Generator) copyStack(src *otmap.Tile, keepAllDecoration bool) *otmap.Tile {
	t := &otmap.Tile{Flags: src.Flags}
	if src.Ground.ID != 0 {
		t.Ground = otmap.Item{ID: src.Ground.ID, Subtype: src.Ground.Subtype}
	}
	for _, it := range src.Items {
		if !keepAllDecoration && !g.a.IsStructural(it.ID) && !g.a.CommonDecoration[it.ID] {
			continue
		}
		t.Items = append(t.Items, otmap.Item{ID: it.ID, Subtype: it.Subtype})
	}
	return t
}

func (g *Generator) tileFromStack(ids []uint16) *otmap.Tile {
	if len(ids) == 0 {
		return nil
	}
	t := &otmap.Tile{}
	for idx, id := range ids {
		if idx == 0 {
			if typ := g.a.DB.Get(id); isGroundType(typ) {
				t.Ground = otmap.Item{ID: id}
				continue
			}
		}
		t.Items = append(t.Items, otmap.Item{ID: id})
	}
	return t
}

func (g *Generator) fromDictionary(key5, x, y int, walk bool) *otmap.Tile {
	cands := g.a.Context5[key5]
	cacheKey := key5
	if len(cands) == 0 {
		key3 := Key3(g.walk, g.w, g.h, x, y)
		cands = g.a.Context3[key3]
		cacheKey = -1 - key3
	}
	if len(cands) == 0 {
		if walk {
			return g.tileFromStack(g.a.FloorStack)
		}
		return g.tileFromStack(g.a.FillStack)
	}
	groups, ok := g.groupCache[cacheKey]
	if !ok {
		bySig := map[string][]int{}
		for _, idx := range cands {
			sig := g.a.StructuralSignature(g.a.Tiles[idx])
			bySig[sig] = append(bySig[sig], idx)
		}
		groups = make([]sigGroup, 0, len(bySig))
		for sig, cells := range bySig {
			groups = append(groups, sigGroup{sig: sig, cells: cells})
		}
		sort.Slice(groups, func(i, j int) bool {
			if len(groups[i].cells) != len(groups[j].cells) {
				return len(groups[i].cells) > len(groups[j].cells)
			}
			return groups[i].sig < groups[j].sig
		})
		g.groupCache[cacheKey] = groups
	}
	pick := groups[0].cells[g.rng.Intn(len(groups[0].cells))]
	return g.copyStack(g.a.Tiles[pick], false)
}

func (g *Generator) spawnCorridors(creatures *[]PlacedCreature, cells []int) {
	var models []otmap.Creature
	for k, cr := range g.a.CreatureAt {
		if g.a.InRun[k] && !cr.NPC {
			models = append(models, cr)
		}
	}
	if len(models) == 0 {
		for _, cr := range g.a.CreatureAt {
			if !cr.NPC {
				models = append(models, cr)
			}
		}
	}
	if len(models) == 0 || len(cells) == 0 {
		return
	}
	target := int(math.Round(float64(len(cells)) * g.a.CorridorCreatureDensity * g.o.SpawnDensity))
	spacing := g.a.CreatureSpacing
	occupied := make(map[int]bool, len(*creatures)+target)
	for _, c := range *creatures {
		occupied[c.Y*g.w+c.X] = true
	}
	order := append([]int(nil), cells...)
	g.rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
	placed := 0
	for _, i := range order {
		if placed >= target {
			break
		}
		x, y := i%g.w, i/g.w
		free := true
		for dy := -spacing + 1; dy < spacing && free; dy++ {
			for dx := -spacing + 1; dx < spacing; dx++ {
				if occupied[(y+dy)*g.w+x+dx] {
					free = false
					break
				}
			}
		}
		if !free {
			continue
		}
		m := models[g.rng.Intn(len(models))]
		*creatures = append(*creatures, PlacedCreature{X: x, Y: y, Z: 0, Creature: m})
		occupied[i] = true
		placed++
	}
}

func (g *Generator) addExtraCreatures(creatures *[]PlacedCreature, tiles map[int]*otmap.Tile) {
	var models []otmap.Creature
	for _, cr := range g.a.CreatureAt {
		if !cr.NPC {
			models = append(models, cr)
		}
	}
	if len(models) == 0 {
		return
	}
	extra := int(float64(len(*creatures)) * (g.o.SpawnDensity - 1.0))
	occupied := make(map[int]bool, len(*creatures)+extra)
	for _, c := range *creatures {
		occupied[c.Y*g.w+c.X] = true
	}
	var walkCells []int
	for i, w := range g.walk {
		if w {
			if _, ok := tiles[i]; ok {
				walkCells = append(walkCells, i)
			}
		}
	}
	if len(walkCells) == 0 {
		return
	}
	for k := 0; k < extra*20 && extra > 0; k++ {
		i := walkCells[g.rng.Intn(len(walkCells))]
		x, y := i%g.w, i/g.w
		free := true
		for dy := -2; dy <= 2 && free; dy++ {
			for dx := -2; dx <= 2; dx++ {
				if occupied[(y+dy)*g.w+x+dx] {
					free = false
					break
				}
			}
		}
		if !free {
			continue
		}
		m := models[g.rng.Intn(len(models))]
		*creatures = append(*creatures, PlacedCreature{X: x, Y: y, Z: 0, Creature: m})
		occupied[i] = true
		extra--
	}
}

func thinCreatures(creatures []PlacedCreature) []PlacedCreature {
	keep := make([]PlacedCreature, 0, len(creatures))
	type key struct{ x, y, z int }
	taken := make(map[key]bool, len(creatures))
	for _, c := range creatures {
		k := key{c.X, c.Y, c.Z}
		if taken[k] {
			continue
		}
		taken[k] = true
		keep = append(keep, c)
	}
	return keep
}

func signInt(v int) int {
	if v > 0 {
		return 1
	}
	if v < 0 {
		return -1
	}
	return 0
}

func modInt(a, m int) int {
	return ((a % m) + m) % m
}
