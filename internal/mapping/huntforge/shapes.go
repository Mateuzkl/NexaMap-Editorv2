package huntforge

import (
	"fmt"
	"math"
	"sort"
)

var autoStyles = []ShapeStyle{
	StyleStar, StyleCross, StyleDiagonal, StyleOctagon,
	StyleArena, StyleBranches, StyleAsymmetric, StyleOrganic,
}

type shapedArm struct {
	sector   int
	group    int
	segments int
	op       *RoomExit
	tip      []RoomExit
	tipX     int
	tipY     int
	built    int
}

type extendResult struct {
	newOpenings []RoomExit
	area        int
	tipX        int
	tipY        int
}

func (g *Generator) generateShaped() *Result {
	style := g.o.Style
	if style == StyleAuto {
		style = autoStyles[g.rng.Intn(len(autoStyles))]
	}
	if style == StyleOrganic || len(g.a.Rooms) == 0 || len(g.a.Runs) == 0 {
		o2 := g.o
		o2.Mode = ModeExpand
		o2.Balanced = true
		r := newGenerator(g.a, o2).generate()
		r.Style = StyleLabel(StyleOrganic)
		r.Report = fmt.Sprintf("Expandir + Novo Estilo [%s]: %s", r.Style, r.Report)
		return r
	}

	w := g.a.CorridorWidth
	sumRoom, roomDiam := 0, 0
	for _, rm := range g.a.Rooms {
		sumRoom += rm.Area()
		if d := max(rm.X2-rm.X1, rm.Y2-rm.Y1) + 1; d > roomDiam {
			roomDiam = d
		}
	}
	avgRoom := float64(sumRoom) / float64(len(g.a.Rooms))
	sumRun := 0
	for _, rn := range g.a.Runs {
		sumRun += rn.Length()
	}
	typical := max(4, sumRun/len(g.a.Runs))
	baseLen := g.clearDist() + 2 + typical
	newArea := math.Max(avgRoom, float64(g.a.WalkArea)*(math.Max(1.0, g.o.AreaFactor)-1.0))

	var arms []*shapedArm
	var branchOffsets []int
	ring := false
	extraFromRing := map[int]bool{}

	switch style {
	case StyleStar:
		for s := 0; s < 8; s++ {
			arms = append(arms, &shapedArm{sector: s, group: s % 2})
		}
	case StyleCross:
		for _, s := range []int{0, 2, 4, 6} {
			arms = append(arms, &shapedArm{sector: s, group: 0})
		}
		branchOffsets = []int{-2, 2}
	case StyleDiagonal:
		for _, s := range []int{1, 3, 5, 7} {
			arms = append(arms, &shapedArm{sector: s, group: 0})
		}
		branchOffsets = []int{-2, 2}
	case StyleOctagon:
		for s := 0; s < 8; s++ {
			arms = append(arms, &shapedArm{sector: s, group: s % 2})
		}
		ring = true
	case StyleArena:
		for s := 0; s < 8; s++ {
			arms = append(arms, &shapedArm{sector: s, group: s % 2})
		}
		ring = true
		for _, s := range []int{0, 2, 4, 6} {
			extraFromRing[s] = true
		}
	case StyleBranches:
		sectors := []int{0, 2, 4, 6}
		if g.rng.Intn(2) == 1 {
			sectors = []int{1, 3, 5, 7}
		}
		if g.rng.Intn(3) == 0 {
			sectors = []int{0, 1, 2, 3, 4, 5, 6, 7}
		}
		for _, s := range sectors {
			arms = append(arms, &shapedArm{sector: s, group: 0})
		}
		branchOffsets = []int{-1, 1}
	case StyleAsymmetric:
		count := 3 + g.rng.Intn(3)
		perm := g.rng.Perm(8)
		for i := 0; i < count; i++ {
			s := perm[i]
			arms = append(arms, &shapedArm{sector: s, group: s})
		}
	}

	segArea := avgRoom + float64(baseLen*w)
	budgetPerArm := newArea / float64(len(arms))
	if ring {
		budgetPerArm -= float64(baseLen * w)
	}
	if len(extraFromRing) > 0 {
		budgetPerArm -= segArea * float64(len(extraFromRing)) / float64(len(arms))
	}
	segments := clampInt(int(math.Round(budgetPerArm/segArea))-len(branchOffsets), 1, 7)
	if ring {
		segments = min(segments, 2)
	}
	groups, maxSeg := 0, 0
	for _, arm := range arms {
		if style == StyleAsymmetric {
			arm.segments = clampInt(segments+g.rng.Intn(3)-1, 1, 8)
		} else {
			arm.segments = segments
		}
		if arm.group+1 > groups {
			groups = arm.group + 1
		}
		if arm.segments+2 > maxSeg {
			maxSeg = arm.segments + 2
		}
	}

	lens := make([][]int, groups)
	for grp := 0; grp < groups; grp++ {
		lens[grp] = make([]int, maxSeg)
		for k := 0; k < maxSeg; k++ {
			lens[grp][k] = baseLen + g.rng.Intn(max(2, typical))
		}
	}
	groupArea := make([]int, groups)

	maxLen := baseLen + typical
	reach := (maxSeg+1)*(maxLen+roomDiam+2) + g.a.FillDepth + 12
	g.initCanvas(g.a.W+2*reach, g.a.H+2*reach)
	target := int(float64(g.a.WalkArea) * math.Max(1.0, g.o.AreaFactor))
	g.target = target
	origin := Point2D{reach, reach}
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
	g.recomputeDistance()
	g.setupEnvelope()
	g.ax, g.ay = 1e9, 1e9

	armCands := map[*shapedArm][]RoomExit{}
	for _, arm := range arms {
		cands := g.findOpeningsToward(arm.sector)
		armCands[arm] = cands
		if len(cands) > 0 {
			op := cands[0]
			arm.op = &op
		}
	}

	for k := 0; k < maxSeg; k++ {
		for _, arm := range arms {
			if k >= arm.segments || arm.op == nil {
				continue
			}
			res := g.extend(*arm.op, lens[arm.group][k], groupArea[arm.group])
			if res == nil && k == 0 {
				cands := armCands[arm]
				for i := 1; i < len(cands) && i <= 5; i++ {
					if res = g.extend(cands[i], lens[arm.group][k], groupArea[arm.group]); res != nil {
						break
					}
				}
			}
			if res == nil {
				arm.op = nil
				continue
			}
			if groupArea[arm.group] == 0 {
				groupArea[arm.group] = res.area
			}
			arm.built++
			arm.tip = res.newOpenings
			arm.tipX = res.tipX
			arm.tipY = res.tipY
			sd := sectorDir[arm.sector]
			arm.op = bestToward(res.newOpenings, float64(sd.X), float64(sd.Y), 0.69)
		}
	}

	if len(branchOffsets) > 0 {
		branchArea := 0
		for _, arm := range arms {
			if arm.built == 0 {
				continue
			}
			for _, off := range branchOffsets {
				sd := sectorDir[modInt(arm.sector+off, 8)]
				op := bestToward(g.activeOpenings(arm.tip), float64(sd.X), float64(sd.Y), 0.69)
				if op == nil {
					continue
				}
				res := g.extend(*op, lens[arm.group][0], branchArea)
				if res != nil && branchArea == 0 {
					branchArea = res.area
				}
			}
		}
	}

	if ring {
		var built []*shapedArm
		for _, arm := range arms {
			if arm.built > 0 {
				built = append(built, arm)
			}
		}
		sort.Slice(built, func(i, j int) bool { return built[i].sector < built[j].sector })
		if len(built) >= 3 {
			for i := 0; i < len(built); i++ {
				a := built[i]
				b := built[(i+1)%len(built)]
				oa := bestToward(g.activeOpenings(a.tip), float64(b.tipX-a.tipX), float64(b.tipY-a.tipY), 0.3)
				ob := bestToward(g.activeOpenings(b.tip), float64(a.tipX-b.tipX), float64(a.tipY-b.tipY), 0.3)
				if oa != nil && ob != nil && g.connectOpenings(*oa, *ob) {
					g.recomputeDistance()
				}
			}
		}
		for _, arm := range built {
			if !extraFromRing[arm.sector] {
				continue
			}
			sd := sectorDir[arm.sector]
			op := bestToward(g.activeOpenings(arm.tip), float64(sd.X), float64(sd.Y), 0.69)
			if op == nil {
				continue
			}
			res := g.extend(*op, lens[arm.group][1], groupArea[arm.group])
			if res == nil {
				continue
			}
			op2 := bestToward(res.newOpenings, float64(sd.X), float64(sd.Y), 0.69)
			if op2 != nil && g.walkCount < target {
				g.extend(*op2, lens[arm.group][2], groupArea[arm.group])
			}
		}
	}

	if float64(g.walkCount) < float64(target)*0.85 {
		x1, y1 := math.MaxInt32, math.MaxInt32
		x2, y2 := 0, 0
		for i, wk := range g.walk {
			if !wk {
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
		radius := float64(max(x2-x1, y2-y1))/2.0*1.05 + float64(w)
		g.ax, g.ay = radius, radius
		g.envScale = 1.0
		g.target = int(float64(target) * 0.95)
		fails := 0
		for g.walkCount < g.target && fails < 600 {
			if g.tryGrow() {
				fails = 0
				g.recomputeDistance()
			} else {
				fails++
				if fails%200 == 0 {
					g.envScale *= 1.06
				}
			}
		}
	}

	if style == StyleAsymmetric || style == StyleBranches {
		want := 1 + g.rng.Intn(2)
		for k := 0; k < 200 && g.loops < want; k++ {
			if g.tryLoop() {
				g.recomputeDistance()
			}
		}
	}

	misses := g.repairMask()
	pruned := g.pruneDisconnected()
	res := g.skin(origin)
	res.RoomsAdded = g.rooms
	res.CorridorsAdded = g.corridors
	res.ContextMisses = misses
	res.Style = StyleLabel(style)
	armsOk := 0
	for _, a := range arms {
		if a.built > 0 {
			armsOk++
		}
	}
	connNote := "tudo conectado"
	if pruned > 0 {
		connNote = fmt.Sprintf("%d tiles soltos removidos", pruned)
	}
	res.Report = fmt.Sprintf(
		"Expandir + Novo Estilo [%s]: %d/%d braços (%d trecho(s) cada), área %d → %d tiles andáveis (%.0f%%), %d salas e %d corredores novos, %d criaturas, %d vizinhanças sem exemplo, %s, seed %d",
		res.Style, armsOk, len(arms), segments, g.a.WalkArea, res.WalkArea,
		float64(res.WalkArea)/float64(max(1, g.a.WalkArea))*100,
		g.rooms, g.corridors, res.Creatures, misses, connNote, g.o.Seed,
	)
	return res
}

func (g *Generator) activeOpenings(list []RoomExit) []RoomExit {
	var out []RoomExit
	for _, op := range list {
		if g.hasOpening(op) {
			out = append(out, op)
		}
	}
	return out
}

func (g *Generator) extend(op RoomExit, length, preferredArea int) *extendResult {
	ignore := -1
	if g.kind[op.Y*g.w+op.X] == kindIdentity {
		ignore = g.a.CorridorWidth*2 + g.clearDist() + 2
	}
	type roomOpt struct {
		room  *RoomShape
		exit  RoomExit
		score int
	}
	var options []roomOpt
	for _, r := range g.a.Rooms {
		for _, e := range r.Exits {
			if e.DX == -op.DX && e.DY == -op.DY {
				sc := g.rng.Intn(1000)
				if preferredArea > 0 {
					sc = absInt(r.Area()-preferredArea) + g.rng.Intn(8)
				}
				options = append(options, roomOpt{r, e, sc})
			}
		}
	}
	if len(options) == 0 {
		return nil
	}
	sort.Slice(options, func(i, j int) bool { return options[i].score < options[j].score })

	lengths := [7]int{length, length * 3 / 4, length * 5 / 4, length * 3 / 2, length * 2, g.clearDist() + 3, length * 3}
	for _, L := range lengths {
		if L <= 0 {
			continue
		}
		var corridor []corridorCell
		ex, ey := g.drawCorridor(op.X, op.Y, op.DX, op.DY, L, &corridor)
		limit := min(8, len(options))
		for i := 0; i < limit; i++ {
			opt := options[i]
			ox, oy := ex-opt.exit.X, ey-opt.exit.Y
			roomSet := make(map[int]bool, len(opt.room.Cells))
			ok := true
			for _, s := range opt.room.Cells {
				x, y := s%g.a.W+ox, s/g.a.W+oy
				if !g.inside(x, y, g.a.FillDepth+2) {
					ok = false
					break
				}
				d := y*g.w + x
				if g.dist[d] < g.clearDist()+1 {
					ok = false
					break
				}
				roomSet[d] = true
			}
			if !ok {
				continue
			}
			if !g.corridorClear(corridor, roomSet, op.X, op.Y, ex, ey, true, ignore) {
				continue
			}
			g.commit(corridor, kindCorridor)
			before := len(g.openings)
			g.placeRoom(opt.room, ox, oy, &opt.exit)
			newOps := append([]RoomExit(nil), g.openings[before:]...)
			g.removeOpening(op)
			g.rooms++
			g.corridors++
			g.recomputeDistance()
			return &extendResult{
				newOpenings: newOps,
				area:        opt.room.Area(),
				tipX:        opt.room.CenterX + ox,
				tipY:        opt.room.CenterY + oy,
			}
		}
	}
	return nil
}

func (g *Generator) findOpeningsToward(sector int) []RoomExit {
	sd := sectorDir[sector]
	slen := math.Sqrt(float64(sd.X*sd.X + sd.Y*sd.Y))
	ux, uy := float64(sd.X)/slen, float64(sd.Y)/slen
	seen := map[RoomExit]bool{}
	var cands []RoomExit
	for t := 0; t < 60; t++ {
		if op, ok := g.findWallOpening(sector, 1.0/3.0); ok && !seen[op] {
			seen[op] = true
			cands = append(cands, op)
		}
	}
	for t := 0; t < 60 && len(cands) < 3; t++ {
		if op, ok := g.findWallOpening(sector, 0.67); ok && !seen[op] {
			seen[op] = true
			cands = append(cands, op)
		}
	}
	score := func(op RoomExit) float64 {
		px, py := float64(op.X)-g.cx, float64(op.Y)-g.cy
		along := px*ux + py*uy
		perp := math.Abs(px*uy - py*ux)
		exact := 0.0
		if op.DX == sd.X && op.DY == sd.Y {
			exact = 1000.0
		}
		return exact + along - 1.5*perp
	}
	sort.Slice(cands, func(i, j int) bool { return score(cands[i]) > score(cands[j]) })
	return cands
}

func bestToward(ops []RoomExit, tx, ty, minCos float64) *RoomExit {
	tl := math.Sqrt(tx*tx + ty*ty)
	if tl < 1e-9 || len(ops) == 0 {
		return nil
	}
	var best *RoomExit
	bestCos := minCos
	for i := range ops {
		op := ops[i]
		ol := math.Sqrt(float64(op.DX*op.DX + op.DY*op.DY))
		cos := (float64(op.DX)*tx + float64(op.DY)*ty) / (ol * tl)
		if cos >= bestCos {
			bestCos = cos
			cp := op
			best = &cp
		}
	}
	return best
}

func (g *Generator) pruneDisconnected() int {
	seen := make([]bool, len(g.walk))
	q := make([]int, 0, g.walkCount)
	for i, wk := range g.walk {
		if wk && g.kind[i] == kindIdentity {
			seen[i] = true
			q = append(q, i)
		}
	}
	if len(q) == 0 {
		return 0
	}
	for head := 0; head < len(q); head++ {
		i := q[head]
		x, y := i%g.w, i/g.w
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				xx, yy := x+dx, y+dy
				if !g.inside(xx, yy, 0) {
					continue
				}
				j := yy*g.w + xx
				if g.walk[j] && !seen[j] {
					seen[j] = true
					q = append(q, j)
				}
			}
		}
	}
	removed := 0
	for i, wk := range g.walk {
		if wk && !seen[i] {
			g.walk[i] = false
			g.walkCount--
			removed++
		}
	}
	return removed
}
