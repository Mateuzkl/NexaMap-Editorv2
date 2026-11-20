package mapedit

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/nekiro/ots-creator/internal/core/otitems"
	"github.com/nekiro/ots-creator/internal/mapping/materials"
	"github.com/nekiro/ots-creator/internal/mapping/otmap"
)

// AutoLearnedBorder represents one ground family and its inferred 12-edge border
// discovered by scanning an entire map.
type AutoLearnedBorder struct {
	Name          string            `json:"name"`
	GroundIDs     []uint16          `json:"groundIds"`
	Align         string            `json:"align"` // "outer" or "inner"
	ToTarget      string            `json:"toTarget"`
	ZOrder        int               `json:"zOrder"`
	Edges         map[string]uint16 `json:"edges"`
	AssignedCount int               `json:"assignedCount"`
	Contacts      int               `json:"contacts"`
	Confidence    float64           `json:"confidence"`
	AlreadyKnown  bool              `json:"alreadyKnown"`
}

// WholeMapBorderReport summarizes an automatic whole-map border discovery scan.
type WholeMapBorderReport struct {
	ScannedTiles    int                 `json:"scannedTiles"`
	DistinctGrounds int                 `json:"distinctGrounds"`
	MissingGrounds  int                 `json:"missingGrounds"`
	Borders         []AutoLearnedBorder `json:"borders"`
}

// LearnBordersFromMap scans every tile on all floors of the open map to
// automatically discover 12-edge borders for ground IDs.
//
// Pass 1 identifies cardinal border items around each ground ID and clusters
// unconfigured ground variations that share the exact same border pieces into
// one ground family. Pass 2 computes exact 8-neighbor bitmasks per merged
// family so corners and diagonals are inferred accurately.
func (e *Editor) LearnBordersFromMap(onlyMissing bool) *WholeMapBorderReport {
	report := &WholeMapBorderReport{}
	if e == nil || e.Map == nil {
		return report
	}

	knownWithBorder := map[uint16]bool{}
	knownBrushName := map[uint16]string{}
	distinctGrounds := map[uint16]bool{}
	missingGrounds := map[uint16]bool{}

	isGroundKnownWithBorder := func(gid uint16) bool {
		if v, ok := knownWithBorder[gid]; ok {
			return v
		}
		has := false
		if e.Brushes != nil {
			if gb := e.Brushes.GroundOf(uint32(gid)); gb != nil {
				knownBrushName[gid] = gb.Name
				has = len(gb.borders) > 0
			}
		}
		knownWithBorder[gid] = has
		return has
	}

	// Candidate border item check (cached per item ID for speed on million-tile worlds).
	const (
		candSkip   uint8 = 0
		candNormal uint8 = 1
		candBorder uint8 = 2
	)
	candClassCache := map[uint16]uint8{}
	classifyCand := func(id uint16) uint8 {
		if id == 0 {
			return candSkip
		}
		if c, ok := candClassCache[id]; ok {
			return c
		}
		id32 := uint32(id)
		it := e.itemType(id32)
		cls := candNormal
		if it != nil {
			if it.IsGround() || it.Flags.Has(otitems.FlagMeta) || it.IsDoor() || it.IsTeleport() ||
				it.Flags.Has(otitems.FlagPickupable) || it.Flags.Has(otitems.FlagStackable) {
				cls = candSkip
			} else if it.TopOrder == borderTopOrder {
				cls = candBorder
			} else if it.TopOrder == bottomTopOrder || it.Flags.Has(otitems.FlagUnpassable) {
				cls = candSkip
			}
		}
		if cls != candSkip && e.Brushes != nil {
			if e.Brushes.isBorder(id32) {
				cls = candBorder
			} else if w, _ := e.Brushes.wallOf(id32); w != nil {
				cls = candSkip
			}
		}
		candClassCache[id] = cls
		return cls
	}

	// Pass 1: gather ground IDs and cardinal border signatures for clustering ground variations.
	// Cardinal neighbors in neighbors8: 1=N, 3=W, 4=E, 6=S -> edgeN, edgeW, edgeE, edgeS
	type dirKey struct {
		gid  uint16
		edge int // 0..3 -> N,E,S,W
	}
	cardinalVotes := map[dirKey]map[uint16]int{}
	groundTileCount := map[uint16]int{}
	// Track pairs of unconfigured grounds that appear directly adjacent on the map
	// so Pass 1 only clusters ground variations that genuinely form the same terrain.
	type gidPair struct{ a, b uint16 }
	adjacentGrounds := map[gidPair]bool{}
	// Also keep tile positions that have candidate border items for blazing-fast Pass 2!
	var borderHostPositions []otmap.Position

	totalTiles := 0
	e.eachTile(false, func(_ otmap.Position, _ *otmap.Tile) { totalTiles++ })

	cardinalDirs := [4]struct {
		ni   int
		slot int
	}{
		{1, 0}, // N
		{4, 1}, // E
		{6, 2}, // S
		{3, 3}, // W
	}

	scanned := 0
	var peekTile otmap.Tile
	var peekBuf []otmap.Item
	e.eachTile(false, func(pos otmap.Position, tile *otmap.Tile) {
		scanned++
		if e.Progress != nil && scanned&0xFFFF == 0 {
			e.Progress(scanned, totalTiles*2)
		}
		if tile.Ground.ID != 0 && tile.Ground.ID <= math.MaxUint16 {
			gid := uint16(tile.Ground.ID)
			distinctGrounds[gid] = true
			groundTileCount[gid]++
			if !isGroundKnownWithBorder(gid) {
				missingGrounds[gid] = true
				// Record adjacency to E and S neighbors (covers all undirected cardinal pairs once).
				for _, off := range [2][2]int{{1, 0}, {0, 1}} {
					nx, ny := int(pos.X)+off[0], int(pos.Y)+off[1]
					if nx <= 0xFFFF && ny <= 0xFFFF {
						if nt := e.Map.Peek(otmap.Position{X: uint16(nx), Y: uint16(ny), Z: pos.Z}, &peekTile, peekBuf[:0]); nt != nil && nt.Ground.ID != 0 && nt.Ground.ID <= math.MaxUint16 && nt.Ground.ID != tile.Ground.ID {
							ngid := uint16(nt.Ground.ID)
							if !isGroundKnownWithBorder(ngid) {
								a, b := gid, ngid
								if a > b {
									a, b = b, a
								}
								adjacentGrounds[gidPair{a, b}] = true
							}
						}
					}
				}
			}
		}
		if len(tile.Items) == 0 {
			return
		}
		var hostCands []uint16
		hasStrictBorder := false
		for _, item := range tile.Items {
			if item.ID > math.MaxUint16 {
				continue
			}
			cid := uint16(item.ID)
			cls := classifyCand(cid)
			if cls == candBorder {
				hasStrictBorder = true
			}
			if cls != candSkip {
				hostCands = append(hostCands, cid)
			}
		}
		if len(hostCands) == 0 {
			return
		}
		// If this tile has explicit TopOrder==1 border items, only consider those
		// to keep noise from loose decorations out.
		if hasStrictBorder {
			filtered := hostCands[:0]
			for _, cid := range hostCands {
				if classifyCand(cid) == candBorder {
					filtered = append(filtered, cid)
				}
			}
			hostCands = filtered
		}

		// Check if any neighbor differs in ground (or is empty) before saving this tile for Pass 2.
		var hostG uint16
		if tile.Ground.ID <= math.MaxUint16 {
			hostG = uint16(tile.Ground.ID)
		}
		hasBoundary := false
		for _, cd := range cardinalDirs {
			off := neighbors8[cd.ni]
			nx, ny := int(pos.X)+off[0], int(pos.Y)+off[1]
			if nx < 0 || ny < 0 || nx > 0xFFFF || ny > 0xFFFF {
				continue
			}
			nt := e.Map.Peek(otmap.Position{X: uint16(nx), Y: uint16(ny), Z: pos.Z}, &peekTile, peekBuf[:0])
			var ng uint16
			if nt != nil && nt.Ground.ID <= math.MaxUint16 {
				ng = uint16(nt.Ground.ID)
			}
			if ng == hostG {
				continue
			}
			hasBoundary = true
			if ng != 0 && hostG != 0 {
				if onlyMissing && isGroundKnownWithBorder(ng) {
					continue
				}
				dk := dirKey{gid: ng, edge: cd.slot}
				m := cardinalVotes[dk]
				if m == nil {
					m = map[uint16]int{}
					cardinalVotes[dk] = m
				}
				for _, cid := range hostCands {
					m[cid]++
				}
			}
		}
		if !hasBoundary {
			// Still check diagonals for corner-only contacts.
			for _, ni := range [4]int{0, 2, 5, 7} {
				off := neighbors8[ni]
				nx, ny := int(pos.X)+off[0], int(pos.Y)+off[1]
				if nx < 0 || ny < 0 || nx > 0xFFFF || ny > 0xFFFF {
					continue
				}
				nt := e.Map.Peek(otmap.Position{X: uint16(nx), Y: uint16(ny), Z: pos.Z}, &peekTile, peekBuf[:0])
				var ng uint16
				if nt != nil && nt.Ground.ID <= math.MaxUint16 {
					ng = uint16(nt.Ground.ID)
				}
				if ng != hostG {
					hasBoundary = true
					break
				}
			}
		}
		if hasBoundary {
			borderHostPositions = append(borderHostPositions, pos)
		}
	})

	report.ScannedTiles = scanned
	report.DistinctGrounds = len(distinctGrounds)
	report.MissingGrounds = len(missingGrounds)

	// Build ground family clustering via Union-Find:
	// - Known brush ground IDs cluster by brush name.
	// - Unconfigured ground IDs that share a top cardinal border item on the same
	//   side (with >= 2 observations) are merged into one ground family!
	parent := map[uint16]uint16{}
	var find func(uint16) uint16
	find = func(x uint16) uint16 {
		p, ok := parent[x]
		if !ok {
			parent[x] = x
			return x
		}
		if p != x {
			parent[x] = find(p)
		}
		return parent[x]
	}
	union := func(a, b uint16) {
		ra, rb := find(a), find(b)
		if ra != rb {
			if ra < rb {
				parent[rb] = ra
			} else {
				parent[ra] = rb
			}
		}
	}

	// Group known brush members first.
	brushFirst := map[string]uint16{}
	for gid := range distinctGrounds {
		find(gid)
		if bname := knownBrushName[gid]; bname != "" {
			if first, ok := brushFirst[bname]; ok {
				union(first, gid)
			} else {
				brushFirst[bname] = gid
			}
		}
	}

	// Cluster unconfigured grounds that share top cardinal border items AND
	// actually appear adjacent on the map (prevents merging unrelated terrains).
	type edgeBorderKey struct {
		edge     int
		borderID uint16
	}
	edgeOwners := map[edgeBorderKey][]uint16{}
	for gid := range distinctGrounds {
		if isGroundKnownWithBorder(gid) {
			continue
		}
		for edge := 0; edge < 4; edge++ {
			votes := cardinalVotes[dirKey{gid: gid, edge: edge}]
			if len(votes) == 0 {
				continue
			}
			var bestID uint16
			bestCount := 0
			totalVotes := 0
			for cid, cnt := range votes {
				totalVotes += cnt
				if cnt > bestCount || (cnt == bestCount && cid < bestID) {
					bestID = cid
					bestCount = cnt
				}
			}
			ratio := float64(bestCount) / float64(max(totalVotes, 1))
			strong := bestCount >= 2 && ratio >= 0.4
			strictSingle := classifyCand(bestID) == candBorder && bestCount >= 1 && ratio >= 0.5
			if bestID != 0 && (strong || strictSingle) {
				k := edgeBorderKey{edge: edge, borderID: bestID}
				edgeOwners[k] = append(edgeOwners[k], gid)
			}
		}
	}
	for _, gids := range edgeOwners {
		if len(gids) < 2 {
			continue
		}
		slices.Sort(gids)
		for i := 0; i < len(gids); i++ {
			for j := i + 1; j < len(gids); j++ {
				if adjacentGrounds[gidPair{gids[i], gids[j]}] {
					union(gids[i], gids[j])
				}
			}
		}
	}

	// Materialize merged families.
	famRootToIdx := map[uint16]int{}
	type groundFamily struct {
		ids          []uint16
		known        bool
		brushName    string
		primaryID    uint16
		primaryCount int
	}
	var families []groundFamily
	gidToFam := map[uint16]int{}

	sortedGids := make([]uint16, 0, len(distinctGrounds))
	for gid := range distinctGrounds {
		sortedGids = append(sortedGids, gid)
	}
	slices.Sort(sortedGids)
	for _, gid := range sortedGids {
		root := find(gid)
		idx, ok := famRootToIdx[root]
		if !ok {
			idx = len(families)
			famRootToIdx[root] = idx
			families = append(families, groundFamily{})
		}
		f := &families[idx]
		f.ids = append(f.ids, gid)
		if isGroundKnownWithBorder(gid) {
			f.known = true
		}
		if f.brushName == "" && knownBrushName[gid] != "" {
			f.brushName = knownBrushName[gid]
		}
		if cnt := groundTileCount[gid]; cnt > f.primaryCount {
			f.primaryCount = cnt
			f.primaryID = gid
		}
		gidToFam[gid] = idx
	}

	// Pass 2: exact 12-edge inference per family (outer against other grounds, inner against void).
	type famAlignKey struct {
		fam   int
		align string // "outer" or "inner"
	}
	type itemVote struct {
		edges   [12]int
		total   int
		strict  bool
		matches int
	}
	type famAcc struct {
		contacts int
		votes    map[uint16]*itemVote
		edgeHits [12]int
	}
	accum := map[famAlignKey]*famAcc{}
	getAcc := func(fam int, align string) *famAcc {
		k := famAlignKey{fam: fam, align: align}
		a := accum[k]
		if a == nil {
			a = &famAcc{votes: map[uint16]*itemVote{}}
			accum[k] = a
		}
		return a
	}

	var hostScratch otmap.Tile
	var hostBuf []otmap.Item
	for idx, pos := range borderHostPositions {
		if e.Progress != nil && idx&0x3FFF == 0 {
			e.Progress(totalTiles+idx*totalTiles/max(len(borderHostPositions), 1), totalTiles*2)
		}
		tile := e.Map.Peek(pos, &hostScratch, hostBuf[:0])
		if tile == nil || tile.Ground.ID == 0 || tile.Ground.ID > math.MaxUint16 {
			continue
		}
		hostFam, ok := gidToFam[uint16(tile.Ground.ID)]
		if !ok {
			continue
		}

		// Extract candidate border items on this tile.
		var cands []uint16
		hasStrict := false
		for _, item := range tile.Items {
			if item.ID > math.MaxUint16 {
				continue
			}
			cid := uint16(item.ID)
			cls := classifyCand(cid)
			if cls == candBorder {
				hasStrict = true
			}
			if cls != candSkip {
				cands = append(cands, cid)
			}
		}
		if len(cands) == 0 {
			continue
		}
		if hasStrict {
			filtered := cands[:0]
			for _, cid := range cands {
				if classifyCand(cid) == candBorder {
					filtered = append(filtered, cid)
				}
			}
			cands = filtered
		}

		// Read 8 neighbor families.
		var nFams [8]int
		emptyMask := 0
		var distinctNeighbors [8]int
		distinctCount := 0
		for ni, off := range neighbors8 {
			nx, ny := int(pos.X)+off[0], int(pos.Y)+off[1]
			nf := -1
			if nx >= 0 && ny >= 0 && nx <= 0xFFFF && ny <= 0xFFFF {
				if nt := e.Map.Peek(otmap.Position{X: uint16(nx), Y: uint16(ny), Z: pos.Z}, &peekTile, peekBuf[:0]); nt != nil && nt.Ground.ID != 0 && nt.Ground.ID <= math.MaxUint16 {
					if fIdx, found := gidToFam[uint16(nt.Ground.ID)]; found {
						nf = fIdx
					}
				}
			}
			nFams[ni] = nf
			if nf == -1 {
				emptyMask |= 1 << ni
			} else if nf != hostFam {
				seen := false
				for j := 0; j < distinctCount; j++ {
					if distinctNeighbors[j] == nf {
						seen = true
						break
					}
				}
				if !seen {
					distinctNeighbors[distinctCount] = nf
					distinctCount++
				}
			}
		}

		recordPieces := func(fam int, align string, pieces []int) {
			if onlyMissing && families[fam].known {
				return
			}
			acc := getAcc(fam, align)
			acc.contacts++
			for _, edge := range pieces {
				if edge >= edgeN && edge <= edgeDSW {
					acc.edgeHits[edge-1]++
				}
			}
			for _, cid := range cands {
				iv := acc.votes[cid]
				if iv == nil {
					iv = &itemVote{strict: classifyCand(cid) == candBorder}
					acc.votes[cid] = iv
				}
				iv.total++
				for _, edge := range pieces {
					if edge >= edgeN && edge <= edgeDSW {
						iv.edges[edge-1]++
					}
				}
			}
		}

		// Outer borders from neighbor families lying onto this tile.
		for j := 0; j < distinctCount; j++ {
			srcFam := distinctNeighbors[j]
			mask := 0
			for ni := 0; ni < 8; ni++ {
				if nFams[ni] == srcFam {
					mask |= 1 << ni
				}
			}
			if pieces := borderPieces(mask); len(pieces) > 0 {
				recordPieces(srcFam, "outer", pieces)
			}
		}

		// Inner border of host family against empty/void tiles.
		if emptyMask != 0 {
			if pieces := borderPieces(emptyMask); len(pieces) > 0 {
				recordPieces(hostFam, "inner", pieces)
			}
		}
	}

	// Convert accumulated votes into clean AutoLearnedBorder entries.
	var results []AutoLearnedBorder
	for k, acc := range accum {
		if acc.contacts < 4 {
			continue
		}
		// Assign each candidate item to its dominant edge slot if its purity is clean.
		type slotWinner struct {
			id    uint16
			score float64
			conf  float64
			obs   int
		}
		var winners [12]slotWinner

		for cid, iv := range acc.votes {
			if iv.total < 2 {
				continue
			}
			bestSlot := -1
			bestCount := 0
			sumCounts := 0
			for s := 0; s < 12; s++ {
				sumCounts += iv.edges[s]
				if iv.edges[s] > bestCount {
					bestCount = iv.edges[s]
					bestSlot = s
				}
			}
			if bestSlot < 0 || bestCount < 2 {
				continue
			}
			// Edge purity: this item must overwhelmingly appear on ONE specific edge direction.
			purity := float64(bestCount) / float64(max(sumCounts, 1))
			if purity < 0.45 {
				continue
			}
			coverage := float64(bestCount) / float64(max(acc.edgeHits[bestSlot], 1))
			if coverage > 1 {
				coverage = 1
			}
			conf := 0.55*purity + 0.45*coverage
			if iv.strict {
				conf = math.Min(1.0, conf+0.12)
			}
			score := conf * math.Log2(float64(bestCount)+1)
			if score > winners[bestSlot].score {
				winners[bestSlot] = slotWinner{id: cid, score: score, conf: conf, obs: bestCount}
			}
		}

		edges := map[string]uint16{}
		confSum := 0.0
		for s := 0; s < 12; s++ {
			if winners[s].id != 0 {
				edges[materials.EdgeNames[s]] = winners[s].id
				confSum += winners[s].conf
			}
		}
		if len(edges) < 2 {
			continue
		}
		fam := families[k.fam]
		name := fam.brushName
		if name == "" {
			if it := e.itemType(uint32(fam.primaryID)); it != nil && strings.TrimSpace(it.Name) != "" {
				name = fmt.Sprintf("%s_%d", sanitizeBrushName(it.Name), fam.primaryID)
			} else {
				name = fmt.Sprintf("ground_%d", fam.primaryID)
			}
		}
		if k.align == "inner" {
			name = name + "_inner"
		}
		toTarget := "all"
		if k.align == "inner" {
			toTarget = "none"
		}
		// Put primary ground ID first so server_lookid uses the most common tile variation.
		gids := make([]uint16, 0, len(fam.ids))
		if fam.primaryID != 0 {
			gids = append(gids, fam.primaryID)
		}
		for _, gid := range fam.ids {
			if gid != fam.primaryID {
				gids = append(gids, gid)
			}
		}
		results = append(results, AutoLearnedBorder{
			Name:          name,
			GroundIDs:     gids,
			Align:         k.align,
			ToTarget:      toTarget,
			ZOrder:        3500,
			Edges:         edges,
			AssignedCount: len(edges),
			Contacts:      acc.contacts,
			Confidence:    confSum / float64(len(edges)),
			AlreadyKnown:  fam.known,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].AssignedCount != results[j].AssignedCount {
			return results[i].AssignedCount > results[j].AssignedCount
		}
		if results[i].Contacts != results[j].Contacts {
			return results[i].Contacts > results[j].Contacts
		}
		return results[i].Name < results[j].Name
	})

	report.Borders = results
	return report
}

func sanitizeBrushName(raw string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(raw)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if r == ' ' || r == '-' || r == '_' {
			if b.Len() > 0 && b.String()[b.Len()-1] != '_' {
				b.WriteByte('_')
			}
		}
	}
	return strings.Trim(b.String(), "_")
}

// FormatMultiLearnedBordersXML renders a complete RME/NexaMap <materials> XML
// file containing all <border>, <brush type="ground">, and a <tileset> listing
// every generated ground brush so it can be imported directly in one step.
func FormatMultiLearnedBordersXML(entries []AutoLearnedBorder, startBorderID uint32) string {
	if startBorderID == 0 {
		startBorderID = 10000
	}
	var b strings.Builder
	b.WriteString("<materials>\n")

	type assignedEntry struct {
		borderID uint32
		entry    AutoLearnedBorder
	}
	assigned := make([]assignedEntry, 0, len(entries))
	nextID := startBorderID
	for _, entry := range entries {
		if len(entry.Edges) == 0 {
			continue
		}
		assigned = append(assigned, assignedEntry{borderID: nextID, entry: entry})
		nextID++
	}

	for _, item := range assigned {
		fmt.Fprintf(&b, "\t<border id=\"%d\">\n", item.borderID)
		for _, edge := range materials.EdgeNames {
			if id := item.entry.Edges[edge]; id != 0 {
				fmt.Fprintf(&b, "\t\t<borderitem edge=\"%s\" item=\"%d\"/>\n", edge, id)
			}
		}
		b.WriteString("\t</border>\n")
	}

	var brushNames []string
	usedNames := map[string]int{}
	for _, item := range assigned {
		e := item.entry
		if len(e.GroundIDs) == 0 {
			continue
		}
		baseName := strings.TrimSpace(e.Name)
		if baseName == "" {
			baseName = fmt.Sprintf("ground_%d", e.GroundIDs[0])
		}
		usedNames[baseName]++
		name := baseName
		if usedNames[baseName] > 1 {
			name = fmt.Sprintf("%s_%d", baseName, usedNames[baseName])
		}
		brushNames = append(brushNames, name)

		align := e.Align
		if align != "inner" {
			align = "outer"
		}
		toTarget := e.ToTarget
		if toTarget == "" {
			if align == "inner" {
				toTarget = "none"
			} else {
				toTarget = "all"
			}
		}
		zOrder := e.ZOrder
		if zOrder <= 0 {
			zOrder = 3500
		}
		b.WriteString("\n")
		escaped := strings.ReplaceAll(name, "\"", "&quot;")
		fmt.Fprintf(&b, "\t<brush name=\"%s\" type=\"ground\" server_lookid=\"%d\" z-order=\"%d\">\n", escaped, e.GroundIDs[0], zOrder)
		for _, gid := range e.GroundIDs {
			if gid != 0 {
				fmt.Fprintf(&b, "\t\t<item id=\"%d\" chance=\"100\"/>\n", gid)
			}
		}
		fmt.Fprintf(&b, "\t\t<border align=\"%s\" to=\"%s\" id=\"%d\"/>\n", align, strings.ReplaceAll(toTarget, "\"", "&quot;"), item.borderID)
		b.WriteString("\t</brush>\n")
	}

	if len(brushNames) > 0 {
		b.WriteString("\n\t<tileset name=\"Auto-Learned Borders\">\n\t\t<terrain>\n")
		for _, name := range brushNames {
			fmt.Fprintf(&b, "\t\t\t<brush name=\"%s\"/>\n", strings.ReplaceAll(name, "\"", "&quot;"))
		}
		b.WriteString("\t\t</terrain>\n\t</tileset>\n")
	}

	b.WriteString("</materials>\n")
	return b.String()
}
