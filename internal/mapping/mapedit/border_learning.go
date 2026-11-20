package mapedit

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/nekiro/ots-creator/internal/core/otitems"
	"github.com/nekiro/ots-creator/internal/mapping/materials"
	"github.com/nekiro/ots-creator/internal/mapping/otmap"
)

// LearnedGroundFamily groups ground items sharing a brush (or a single unconfigured ground ID).
type LearnedGroundFamily struct {
	Index            int      `json:"index"`
	Name             string   `json:"name"`
	KnownBrush       bool     `json:"knownBrush"`
	RepresentativeID uint16   `json:"representativeId"`
	ItemIDs          []uint16 `json:"itemIds"`
}

// LearnedBorderCandidate represents one scored candidate item for a border edge slot.
type LearnedBorderCandidate struct {
	ItemID       uint16  `json:"itemId"`
	Observations int     `json:"observations"`
	Confidence   float64 `json:"confidence"`
	Purity       float64 `json:"purity"`
	KnownBorder  bool    `json:"knownBorder"`
}

// LearnedBorderSlot holds the winning candidate and alternatives for one of the 12 canonical edges.
type LearnedBorderSlot struct {
	Edge         string                   `json:"edge"`
	ItemID       uint16                   `json:"itemId"`
	Observations int                      `json:"observations"`
	Confidence   float64                  `json:"confidence"`
	Ambiguous    bool                     `json:"ambiguous"`
	Alternatives []LearnedBorderCandidate `json:"alternatives,omitempty"`
}

// LearnedBorderTransition summarizes inferred border edges for one ground-to-ground (or ground-to-empty) transition.
// FamilyB == -1 denotes empty/void tiles.
type LearnedBorderTransition struct {
	FamilyA           int                 `json:"familyA"`
	FamilyB           int                 `json:"familyB"`
	Label             string              `json:"label"`
	Align             string              `json:"align"` // "outer" or "inner"
	Contacts          int                 `json:"contacts"`
	Slots             []LearnedBorderSlot `json:"slots"`
	AssignedCount     int                 `json:"assignedCount"`
	OverallConfidence float64             `json:"overallConfidence"`
	MatchRate         float64             `json:"matchRate"`
}

// LearnedBorderReport is the full border-learning analysis of the active selection.
type LearnedBorderReport struct {
	Floor         int                       `json:"floor"`
	SelectedTiles int                       `json:"selectedTiles"`
	Families      []LearnedGroundFamily     `json:"families"`
	Transitions   []LearnedBorderTransition `json:"transitions"`
}

// LearnBorderFromSelection inspects the selected tiles on the dominant selected floor
// and infers canonical 12-edge auto-borders for every ground transition present.
func (e *Editor) LearnBorderFromSelection() (*LearnedBorderReport, error) {
	selected := e.Selected()
	if len(selected) == 0 {
		return nil, errors.New("select an area with ground and border tiles first")
	}

	// Pick the floor with the most selected tiles.
	floorCounts := map[uint8]int{}
	for _, pos := range selected {
		floorCounts[pos.Z]++
	}
	bestFloor := selected[0].Z
	for z, count := range floorCounts {
		if count > floorCounts[bestFloor] || (count == floorCounts[bestFloor] && z < bestFloor) {
			bestFloor = z
		}
	}

	var tilesPos []otmap.Position
	selectedSet := map[otmap.Position]bool{}
	for _, pos := range selected {
		if pos.Z == bestFloor {
			tilesPos = append(tilesPos, pos)
			selectedSet[pos] = true
		}
	}
	sort.Slice(tilesPos, func(i, j int) bool {
		if tilesPos[i].Y != tilesPos[j].Y {
			return tilesPos[i].Y < tilesPos[j].Y
		}
		return tilesPos[i].X < tilesPos[j].X
	})

	type capturedItem struct {
		id         uint16
		stackIndex int
		isBorder   bool
		topOrder   uint8
		technical  bool
	}
	type capturedTile struct {
		pos          otmap.Position
		groundID     uint16
		family       int
		items        []capturedItem
		neighborFams [8]int
	}

	var families []LearnedGroundFamily
	familyIndex := map[string]int{}
	familyForGround := func(groundID uint16) int {
		if groundID == 0 {
			return -1
		}
		var key, name string
		known := false
		if e.Brushes != nil {
			if gb := e.Brushes.GroundOf(groundID); gb != nil {
				key = "brush:" + strings.ToLower(gb.Name)
				name = gb.Name
				known = true
			}
		}
		if key == "" {
			key = fmt.Sprintf("item:%d", groundID)
			if it := e.itemType(groundID); it != nil && it.Name != "" {
				name = fmt.Sprintf("%s (%d)", it.Name, groundID)
			} else {
				name = fmt.Sprintf("Ground %d", groundID)
			}
		}
		idx, ok := familyIndex[key]
		if !ok {
			idx = len(families)
			familyIndex[key] = idx
			families = append(families, LearnedGroundFamily{
				Index:            idx,
				Name:             name,
				KnownBrush:       known,
				RepresentativeID: groundID,
			})
		}
		if !slices.Contains(families[idx].ItemIDs, groundID) {
			families[idx].ItemIDs = append(families[idx].ItemIDs, groundID)
		}
		return idx
	}

	captured := make([]capturedTile, 0, len(tilesPos))
	for _, pos := range tilesPos {
		tile := e.Map.Tile(pos)
		if tile == nil {
			continue
		}
		ct := capturedTile{
			pos:      pos,
			groundID: tile.Ground.ID,
			family:   familyForGround(tile.Ground.ID),
		}
		for idx, item := range tile.Items {
			it := e.itemType(item.ID)
			var top uint8
			technical := false
			isBorder := false
			if it != nil {
				top = it.TopOrder
				technical = it.Flags.Has(otitems.FlagMeta) || it.IsDoor() || it.IsTeleport()
				isBorder = top == borderTopOrder
			}
			if e.Brushes != nil {
				if e.Brushes.isBorder(item.ID) {
					isBorder = true
				}
				if w, _ := e.Brushes.wallOf(item.ID); w != nil {
					technical = true
				}
			}
			ct.items = append(ct.items, capturedItem{
				id:         item.ID,
				stackIndex: idx,
				isBorder:   isBorder,
				topOrder:   top,
				technical:  technical,
			})
		}
		for ni, offset := range neighbors8 {
			nx, ny := int(pos.X)+offset[0], int(pos.Y)+offset[1]
			if nx < 0 || ny < 0 || nx > 0xFFFF || ny > 0xFFFF {
				ct.neighborFams[ni] = -1
				continue
			}
			npos := otmap.Position{X: uint16(nx), Y: uint16(ny), Z: pos.Z}
			if nt := e.Map.Tile(npos); nt != nil {
				ct.neighborFams[ni] = familyForGround(nt.Ground.ID)
			} else {
				ct.neighborFams[ni] = -1
			}
		}
		captured = append(captured, ct)
	}

	for i := range families {
		slices.Sort(families[i].ItemIDs)
	}

	// Count cardinal contacts between distinct families (including family <-> empty (-1)).
	type famPair struct{ a, b int }
	contacts := map[famPair]int{}
	cardinalIdx := [4]int{1, 3, 4, 6} // N, W, E, S in neighbors8
	for _, ct := range captured {
		if ct.family < 0 {
			continue
		}
		for _, ni := range cardinalIdx {
			nf := ct.neighborFams[ni]
			if nf == ct.family {
				continue
			}
			if nf >= 0 {
				a, b := ct.family, nf
				if a > b {
					a, b = b, a
				}
				contacts[famPair{a, b}]++
			} else {
				contacts[famPair{ct.family, -1}]++
			}
		}
	}

	// Evaluate candidate border transitions in both directions for ground-to-ground
	// (outer border of A lying on B, outer border of B lying on A) and inner border
	// against empty tiles (lying on A when adjacent to empty).
	type candStats struct {
		edgeCounts   [12]int
		boundaryOcc  int
		totalOcc     int
		knownBorder  bool
		borderTopOcc int
	}

	totalItemOcc := map[uint16]int{}
	for _, ct := range captured {
		for _, it := range ct.items {
			totalItemOcc[it.id]++
		}
	}

	inferDirection := func(sourceFam, hostFam int, align string, contactCount int) LearnedBorderTransition {
		statsByID := map[uint16]*candStats{}
		type obs struct {
			edges []int
			items []uint16
		}
		var observations []obs

		for _, ct := range captured {
			if ct.family != hostFam {
				continue
			}
			mask := 0
			for ni := 0; ni < 8; ni++ {
				if ct.neighborFams[ni] == sourceFam {
					mask |= 1 << ni
				}
			}
			if mask == 0 {
				continue
			}
			pieces := borderPieces(mask)
			if len(pieces) == 0 {
				continue
			}
			var candIDs []uint16
			for _, it := range ct.items {
				if it.technical || it.id == 0 {
					continue
				}
				st := statsByID[it.id]
				if st == nil {
					st = &candStats{totalOcc: totalItemOcc[it.id]}
					statsByID[it.id] = st
				}
				st.boundaryOcc++
				st.knownBorder = st.knownBorder || it.isBorder
				if it.topOrder == borderTopOrder {
					st.borderTopOcc++
				}
				for _, edge := range pieces {
					if edge >= edgeN && edge <= edgeDSW {
						st.edgeCounts[edge-1]++
					}
				}
				candIDs = append(candIDs, it.id)
			}
			if len(candIDs) > 0 {
				observations = append(observations, obs{edges: pieces, items: candIDs})
			}
		}

		slots := make([]LearnedBorderSlot, 12)
		for i := 0; i < 12; i++ {
			slots[i].Edge = materials.EdgeNames[i]
		}

		for id, st := range statsByID {
			if st.boundaryOcc == 0 {
				continue
			}
			roleTotal := 0
			bestCount := 0
			var bestEdges []int
			for slotIdx := 0; slotIdx < 12; slotIdx++ {
				c := st.edgeCounts[slotIdx]
				roleTotal += c
				if c > bestCount {
					bestCount = c
					bestEdges = []int{slotIdx}
				} else if c > 0 && c == bestCount {
					bestEdges = append(bestEdges, slotIdx)
				}
			}
			if bestCount == 0 || len(bestEdges) != 1 {
				continue
			}
			bestSlot := bestEdges[0]
			purity := float64(bestCount) / float64(max(roleTotal, 1))
			boundaryRate := float64(st.boundaryOcc) / float64(max(st.totalOcc, 1))
			sampleScore := math.Min(1.0, float64(bestCount)/4.0)
			conf := purity * (0.60 + 0.40*sampleScore) * boundaryRate
			if st.knownBorder {
				conf += 0.08
			}
			if st.borderTopOcc*2 >= st.boundaryOcc {
				conf += 0.04
			}
			conf = math.Min(1.0, math.Max(0.0, conf))
			slots[bestSlot].Alternatives = append(slots[bestSlot].Alternatives, LearnedBorderCandidate{
				ItemID:       id,
				Observations: bestCount,
				Confidence:   math.Round(conf*1000) / 1000,
				Purity:       math.Round(purity*1000) / 1000,
				KnownBorder:  st.knownBorder,
			})
		}

		assigned := 0
		confSum := 0.0
		edgeItem := map[int]uint16{}
		for i := range slots {
			alts := slots[i].Alternatives
			sort.Slice(alts, func(a, b int) bool {
				if alts[a].Confidence != alts[b].Confidence {
					return alts[a].Confidence > alts[b].Confidence
				}
				if alts[a].Observations != alts[b].Observations {
					return alts[a].Observations > alts[b].Observations
				}
				return alts[a].ItemID < alts[b].ItemID
			})
			slots[i].Alternatives = alts
			if len(alts) > 0 {
				slots[i].ItemID = alts[0].ItemID
				slots[i].Observations = alts[0].Observations
				slots[i].Confidence = alts[0].Confidence
				if len(alts) > 1 && math.Abs(alts[0].Confidence-alts[1].Confidence) < 0.08 {
					slots[i].Ambiguous = true
				}
				assigned++
				confSum += alts[0].Confidence
				edgeItem[i+1] = alts[0].ItemID
			}
		}

		// Validate how many observed boundary roles are matched by the learned slots.
		matchedRoles, totalRoles := 0, 0
		for _, o := range observations {
			for _, edge := range o.edges {
				want := edgeItem[edge]
				if want == 0 {
					continue
				}
				totalRoles++
				if slices.Contains(o.items, want) {
					matchedRoles++
				}
			}
		}
		matchRate := 0.0
		if totalRoles > 0 {
			matchRate = math.Round(float64(matchedRoles)/float64(totalRoles)*1000) / 1000
		}
		overall := 0.0
		if assigned > 0 {
			overall = math.Round((confSum/float64(assigned))*1000) / 1000
		}

		label := ""
		if align == "inner" {
			label = fmt.Sprintf("%s → empty (inner border)", families[hostFam].Name)
		} else {
			label = fmt.Sprintf("%s → %s (outer border)", families[sourceFam].Name, families[hostFam].Name)
		}

		famA, famB := sourceFam, hostFam
		if align == "inner" {
			famA, famB = hostFam, -1
		}
		return LearnedBorderTransition{
			FamilyA:           famA,
			FamilyB:           famB,
			Label:             label,
			Align:             align,
			Contacts:          contactCount,
			Slots:             slots,
			AssignedCount:     assigned,
			OverallConfidence: overall,
			MatchRate:         matchRate,
		}
	}

	var transitions []LearnedBorderTransition
	for pair, count := range contacts {
		if pair.b == -1 {
			tr := inferDirection(-1, pair.a, "inner", count)
			if tr.AssignedCount > 0 {
				transitions = append(transitions, tr)
			}
			continue
		}
		trAB := inferDirection(pair.a, pair.b, "outer", count)
		trBA := inferDirection(pair.b, pair.a, "outer", count)
		if trAB.AssignedCount > 0 {
			transitions = append(transitions, trAB)
		}
		if trBA.AssignedCount > 0 {
			transitions = append(transitions, trBA)
		}
	}

	sort.Slice(transitions, func(i, j int) bool {
		if transitions[i].AssignedCount != transitions[j].AssignedCount {
			return transitions[i].AssignedCount > transitions[j].AssignedCount
		}
		if transitions[i].OverallConfidence != transitions[j].OverallConfidence {
			return transitions[i].OverallConfidence > transitions[j].OverallConfidence
		}
		return transitions[i].Contacts > transitions[j].Contacts
	})

	return &LearnedBorderReport{
		Floor:         int(bestFloor),
		SelectedTiles: len(captured),
		Families:      families,
		Transitions:   transitions,
	}, nil
}

// FormatLearnedBorderXML renders a standalone RME-compatible <materials> XML
// containing the learned <border> and (when groundIDs are provided) <brush>.
func FormatLearnedBorderXML(border materials.Border, brushName string, groundIDs []uint16, align, toTarget string, zOrder int) string {
	var b strings.Builder
	b.WriteString("<materials>\n")
	fmt.Fprintf(&b, "\t<border id=\"%d\"", border.ID)
	if border.Group != 0 {
		fmt.Fprintf(&b, " group=\"%d\"", border.Group)
	}
	if border.Optional {
		b.WriteString(" type=\"optional\"")
	}
	b.WriteString(">\n")
	for _, edge := range materials.EdgeNames {
		if id := border.Edges[edge]; id != 0 {
			fmt.Fprintf(&b, "\t\t<borderitem edge=\"%s\" item=\"%d\"/>\n", edge, id)
		}
	}
	b.WriteString("\t</border>\n")

	brushName = strings.TrimSpace(brushName)
	if brushName != "" && len(groundIDs) > 0 {
		if align != "inner" {
			align = "outer"
		}
		if toTarget == "" {
			if align == "inner" {
				toTarget = "none"
			} else {
				toTarget = "all"
			}
		}
		if zOrder <= 0 {
			zOrder = 3500
		}
		b.WriteString("\n")
		escaped := strings.ReplaceAll(brushName, "\"", "&quot;")
		fmt.Fprintf(&b, "\t<brush name=\"%s\" type=\"ground\" server_lookid=\"%d\" z-order=\"%d\">\n", escaped, groundIDs[0], zOrder)
		for _, gid := range groundIDs {
			fmt.Fprintf(&b, "\t\t<item id=\"%d\" chance=\"100\"/>\n", gid)
		}
		fmt.Fprintf(&b, "\t\t<border align=\"%s\" to=\"%s\" id=\"%d\"/>\n", align, strings.ReplaceAll(toTarget, "\"", "&quot;"), border.ID)
		b.WriteString("\t</brush>\n")
	}
	b.WriteString("</materials>\n")
	return b.String()
}
