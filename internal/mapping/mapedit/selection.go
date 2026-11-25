package mapedit

import (
	"errors"
	"fmt"
	"slices"

	"github.com/nekiro/ots-creator/internal/mapping/otmap"
)

func floorRange(z int, mode string) (int, int) {
	z = min(max(z, 0), otmap.MaxFloor)
	switch mode {
	case FloorsLower:
		if z <= otmap.GroundFloor {
			return z, otmap.GroundFloor
		}
		return z, min(otmap.MaxFloor, z+lowerFloorSpan)
	case FloorsVisible:
		if z <= otmap.GroundFloor {
			return 0, otmap.GroundFloor
		}
		return z, min(otmap.MaxFloor, z+lowerFloorSpan)
	}
	return z, z
}

func (e *Editor) syncSelection(pos otmap.Position) {
	if e.selectedPositions == nil {
		e.selectedPositions = map[otmap.Position]struct{}{}
	}
	if e.peekPos(pos).Selected() {
		e.selectedPositions[pos] = struct{}{}
	} else {
		delete(e.selectedPositions, pos)
	}
}

func (e *Editor) RebuildSelection() {
	e.selectedPositions = map[otmap.Position]struct{}{}
	e.selectionFloor = -1
	e.Map.EachStored(func(pos otmap.Position, tile *otmap.Tile) {
		if tile.Selected() {
			e.selectedPositions[pos] = struct{}{}
		}
	})
}

func (e *Editor) Selected() []otmap.Position {
	positions := make([]otmap.Position, 0, len(e.selectedPositions))
	for pos := range e.selectedPositions {
		positions = append(positions, pos)
	}
	slices.SortFunc(positions, otmap.ComparePositions)
	return positions
}

func (e *Editor) SelectionCount() int { return len(e.selectedPositions) }

func (e *Editor) SelectionBounds() (lowest, highest otmap.Position, ok bool) {
	first := true
	for pos := range e.selectedPositions {
		if first {
			lowest, highest, first = pos, pos, false
			continue
		}
		lowest.X, lowest.Y, lowest.Z = min(lowest.X, pos.X), min(lowest.Y, pos.Y), min(lowest.Z, pos.Z)
		highest.X, highest.Y, highest.Z = max(highest.X, pos.X), max(highest.Y, pos.Y), max(highest.Z, pos.Z)
	}
	return lowest, highest, !first
}

func (e *Editor) ClearSelection() {
	e.selectionFloor = -1
	if len(e.selectedPositions) == 0 {
		return
	}
	tx := e.BeginSelection()
	e.deselectAll(tx)
	tx.Commit()
}

func (e *Editor) deselectAll(tx *Tx) {
	for _, pos := range e.Selected() {
		tx.Edit(pos).SetSelected(false)
	}
}

// SelectRect selects the tiles of a rectangle. add keeps the selection,
// remove deselects instead. compensate aligns other floors with how the view
// draws them in perspective (tile (x, y, z) on camera floor z aligns on screen
// with tile (x + z - floor, y + z - floor, floor)).
func (e *Editor) SelectRect(area Rect, floors string, add, remove, compensate bool) {
	tx := e.BeginSelection()
	if !add && !remove {
		e.deselectAll(tx)
	}
	x0, y0, x1, y1 := area.bounds()
	z := min(max(int(area.From.Z), 0), otmap.MaxFloor)
	firstFloor, lastFloor := floorRange(z, floors)
	for floor := firstFloor; floor <= lastFloor; floor++ {
		shift := 0
		if compensate {
			shift = z - floor
		}
		e.Map.EachIn(x0+shift, y0+shift, x1+shift, y1+shift, floor, floor, func(pos otmap.Position, tile *otmap.Tile) {
			if tile.Selected() == !remove && (remove || allSelected(tile)) {
				return
			}
			tx.Edit(pos).SetSelected(!remove)
		})
	}
	tx.Commit()
	if len(e.selectedPositions) == 0 {
		e.selectionFloor = -1
	} else if !add && !remove {
		e.selectionFloor = z
	} else if add && e.selectionFloor < 0 {
		e.selectionFloor = z
	}
}

func allSelected(tile *otmap.Tile) bool {
	if tile.Ground.ID != 0 && !tile.Ground.Selected {
		return false
	}
	for i := range tile.Items {
		if !tile.Items[i].Selected {
			return false
		}
	}
	if tile.Ext != nil {
		for i := range tile.Ext.Creatures {
			if !tile.Ext.Creatures[i].Selected {
				return false
			}
		}
		if tile.Ext.Spawn != nil && !tile.Ext.Spawn.Selected {
			return false
		}
		if tile.Ext.SpawnNPC != nil && !tile.Ext.SpawnNPC.Selected {
			return false
		}
	}
	return true
}

// SelectAt selects the top thing of a tile: a creature, the spawn, or the
// top item. toggle adds or removes it from the selection instead of
// replacing it. It returns what was selected ("creature", "spawn", "item"
// or "").
func (e *Editor) SelectAt(target otmap.Position, toggle bool, showCreatures, showSpawns bool) string {
	tile := e.Map.Tile(target)
	tx := e.BeginSelection()
	defer func() {
		tx.Commit()
		if len(e.selectedPositions) == 0 {
			e.selectionFloor = -1
		} else if !toggle || e.selectionFloor < 0 {
			e.selectionFloor = int(target.Z)
		}
	}()
	if !toggle {
		for _, other := range e.Selected() {
			if other != target {
				tx.Edit(other).SetSelected(false)
			}
		}
	}
	if tile == nil {
		return ""
	}
	edited := tx.Edit(target)
	if !toggle {
		edited.SetSelected(false)
	}
	if showCreatures && len(edited.Creatures()) > 0 {
		creature := &edited.Ext.Creatures[len(edited.Ext.Creatures)-1]
		creature.Selected = !toggle || !creature.Selected
		return "creature"
	}
	if showSpawns && edited.Spawn() != nil {
		edited.Ext.Spawn.Selected = !toggle || !edited.Ext.Spawn.Selected
		return "spawn"
	}
	if showSpawns && edited.SpawnNPC() != nil {
		edited.Ext.SpawnNPC.Selected = !toggle || !edited.Ext.SpawnNPC.Selected
		return "spawn"
	}
	if itemCount := len(edited.Items); itemCount > 0 {
		topItem := &edited.Items[itemCount-1]
		topItem.Selected = !toggle || !topItem.Selected
		return "item"
	}
	if edited.Ground.ID != 0 {
		edited.Ground.Selected = !toggle || !edited.Ground.Selected
		return "item"
	}
	return ""
}

// SelectItem selects one item of a tile (index -1 is the ground).
func (e *Editor) SelectItem(pos otmap.Position, index int, add bool) {
	tx := e.BeginSelection()
	defer func() {
		tx.Commit()
		if len(e.selectedPositions) == 0 {
			e.selectionFloor = -1
		} else if !add || e.selectionFloor < 0 {
			e.selectionFloor = int(pos.Z)
		}
	}()
	if !add {
		e.deselectAll(tx)
	}
	if e.Map.Tile(pos) == nil {
		return
	}
	edited := tx.Edit(pos)
	if index < 0 {
		edited.Ground.Selected = edited.Ground.ID != 0
	} else if index < len(edited.Items) {
		edited.Items[index].Selected = true
	}
}

func (e *Editor) SelectTiles(positions []otmap.Position, add bool) {
	tx := e.BeginSelection()
	if !add {
		e.deselectAll(tx)
		e.selectionFloor = -1
	}
	for _, pos := range positions {
		if e.Map.Tile(pos) != nil {
			tx.Edit(pos).SetSelected(true)
		}
	}
	tx.Commit()
	if len(e.selectedPositions) == 0 {
		e.selectionFloor = -1
	}
}

type ClipTile struct {
	DX        int              `json:"dx"`
	DY        int              `json:"dy"`
	DZ        int              `json:"dz"`
	Ground    *otmap.Item      `json:"ground,omitempty"`
	Items     []otmap.Item     `json:"items,omitempty"`
	Creatures []otmap.Creature `json:"creatures,omitempty"`
	Spawn     *otmap.Spawn     `json:"spawn,omitempty"`
	SpawnNPC  *otmap.Spawn     `json:"spawnNpc,omitempty"`
	HouseID   uint32           `json:"houseId,omitempty"`
	Flags     uint32           `json:"flags,omitempty"`
}

type Clip struct {
	Tiles      []ClipTile `json:"tiles"`
	TotalTiles int        `json:"totalTiles"`
	Width      int        `json:"width"`
	Height     int        `json:"height"`
	OriginZ    int        `json:"originZ"`
	MinDX      int        `json:"minDx"`
	MaxDX      int        `json:"maxDx"`
	MinDY      int        `json:"minDy"`
	MaxDY      int        `json:"maxDy"`
	MinDZ      int        `json:"minDz"`
	MaxDZ      int        `json:"maxDz"`
}

// Preview returns a copy of Clip suitable for IPC previews. When maxTiles <= 0
// or len(Tiles) > maxTiles, Tiles is omitted while TotalTiles and all bounding
// offsets remain exact so huge clipboards transfer in O(1) bytes.
func (c *Clip) Preview(maxTiles int) *Clip {
	if c == nil {
		return nil
	}
	total := c.TotalTiles
	if total == 0 {
		total = len(c.Tiles)
	}
	preview := *c
	preview.TotalTiles = total
	if maxTiles <= 0 || len(c.Tiles) > maxTiles {
		preview.Tiles = nil
	}
	return &preview
}

func extract(tile *otmap.Tile, take bool) ClipTile {
	var clipTile ClipTile
	if tile.Ground.ID != 0 && tile.Ground.Selected {
		ground := tile.Ground.Clone()
		ground.Selected = false
		clipTile.Ground = &ground
		if take {
			tile.Ground = otmap.Item{}
		}
	}
	if len(tile.Items) > 0 {
		var kept []otmap.Item
		if take {
			kept = tile.Items[:0]
		}
		for _, item := range tile.Items {
			if item.Selected {
				if clipTile.Items == nil {
					clipTile.Items = make([]otmap.Item, 0, len(tile.Items))
				}
				copied := item.Clone()
				copied.Selected = false
				clipTile.Items = append(clipTile.Items, copied)
				if take {
					continue
				}
			}
			if take {
				kept = append(kept, item)
			}
		}
		if take {
			if len(kept) == 0 {
				tile.Items = nil
			} else {
				tile.Items = kept
			}
		}
	}
	if tile.Ext != nil {
		extractExtras(tile, &clipTile, take)
	}
	// A whole tile selection carries the house and flags too.
	if clipTile.Ground != nil {
		clipTile.HouseID, clipTile.Flags = tile.HouseID, tile.Flags
	}
	return clipTile
}

func extractExtras(tile *otmap.Tile, clipTile *ClipTile, take bool) {
	if len(tile.Ext.Creatures) > 0 {
		var keptCreatures []otmap.Creature
		if take {
			keptCreatures = tile.Ext.Creatures[:0]
		}
		for _, creature := range tile.Ext.Creatures {
			if creature.Selected {
				if clipTile.Creatures == nil {
					clipTile.Creatures = make([]otmap.Creature, 0, len(tile.Ext.Creatures))
				}
				creature.Selected = false
				clipTile.Creatures = append(clipTile.Creatures, creature)
				if take {
					continue
				}
			}
			if take {
				keptCreatures = append(keptCreatures, creature)
			}
		}
		if take {
			if len(keptCreatures) == 0 {
				tile.Ext.Creatures = nil
			} else {
				tile.Ext.Creatures = keptCreatures
			}
		}
	}
	if spawn := selectedSpawnCopy(tile.Ext.Spawn); spawn != nil {
		clipTile.Spawn = spawn
		if take {
			tile.Ext.Spawn = nil
		}
	}
	if spawn := selectedSpawnCopy(tile.Ext.SpawnNPC); spawn != nil {
		clipTile.SpawnNPC = spawn
		if take {
			tile.Ext.SpawnNPC = nil
		}
	}
	if take {
		tile.Compact()
	}
}

func selectedSpawnCopy(spawn *otmap.Spawn) *otmap.Spawn {
	if spawn == nil || !spawn.Selected {
		return nil
	}
	copied := *spawn
	copied.Selected = false
	return &copied
}

func (e *Editor) put(tile *otmap.Tile, clipTile ClipTile, selected bool) {
	var preservedGround otmap.Item
	if clipTile.Ground != nil {
		ground := clipTile.Ground.Clone()
		ground.Selected = selected
		tile.Ground = ground
		preservedGround = ground
		if clipTile.HouseID != 0 || clipTile.Flags != 0 {
			tile.HouseID, tile.Flags = clipTile.HouseID, clipTile.Flags
		}
	}
	for _, item := range clipTile.Items {
		copied := item.Clone()
		copied.Selected = selected
		e.addItem(tile, copied)
	}
	if clipTile.Ground != nil {
		// Keep the explicitly copied ground even if a copied item declares a groundEquivalent.
		tile.Ground = preservedGround
	}
	for _, creature := range clipTile.Creatures {
		creature.Selected = selected
		tile.EnsureExt().Creatures = append(tile.EnsureExt().Creatures, creature)
	}
	if clipTile.Spawn != nil {
		spawn := *clipTile.Spawn
		spawn.Selected = selected
		tile.EnsureExt().Spawn = &spawn
	}
	if clipTile.SpawnNPC != nil {
		spawn := *clipTile.SpawnNPC
		spawn.Selected = selected
		tile.EnsureExt().SpawnNPC = &spawn
	}
}

func (c *ClipTile) empty() bool {
	return c.Ground == nil && len(c.Items) == 0 && len(c.Creatures) == 0 && c.Spawn == nil && c.SpawnNPC == nil
}

func (e *Editor) Copy() *Clip {
	lowest, _, ok := e.SelectionBounds()
	if !ok {
		return nil
	}
	anchorZ := int(lowest.Z)
	if e.selectionFloor >= 0 && e.selectionFloor <= otmap.MaxFloor {
		anchorZ = e.selectionFloor
	}
	return e.copyWithAnchor(anchorZ)
}

// CopyAt copies the selection using anchorZ as the reference floor (DZ = tile.Z - anchorZ).
// Copying floors 0..7 from anchorZ=7 produces DZ in -7..0 so pasting at Z=7 keeps floors 0..7.
func (e *Editor) CopyAt(anchorZ int) *Clip {
	if anchorZ < 0 || anchorZ > otmap.MaxFloor {
		return e.Copy()
	}
	return e.copyWithAnchor(anchorZ)
}

func (e *Editor) copyWithAnchor(anchorZ int) *Clip {
	lowest, highest, ok := e.SelectionBounds()
	if !ok {
		return nil
	}
	selected := e.Selected()
	clip := &Clip{
		Tiles:   make([]ClipTile, 0, len(selected)),
		Width:   int(highest.X-lowest.X) + 1,
		Height:  int(highest.Y-lowest.Y) + 1,
		OriginZ: anchorZ,
	}
	first := true
	for _, pos := range selected {
		// peekPos unpacks into scratch with zero heap allocation; extract clones every copied field.
		tile := e.peekPos(pos)
		if tile == nil {
			continue
		}
		clipTile := extract(tile, false)
		if clipTile.empty() {
			continue
		}
		clipTile.DX = int(pos.X - lowest.X)
		clipTile.DY = int(pos.Y - lowest.Y)
		clipTile.DZ = int(pos.Z) - anchorZ
		if first {
			clip.MinDX, clip.MaxDX = clipTile.DX, clipTile.DX
			clip.MinDY, clip.MaxDY = clipTile.DY, clipTile.DY
			clip.MinDZ, clip.MaxDZ = clipTile.DZ, clipTile.DZ
			first = false
		} else {
			clip.MinDX = min(clip.MinDX, clipTile.DX)
			clip.MaxDX = max(clip.MaxDX, clipTile.DX)
			clip.MinDY = min(clip.MinDY, clipTile.DY)
			clip.MaxDY = max(clip.MaxDY, clipTile.DY)
			clip.MinDZ = min(clip.MinDZ, clipTile.DZ)
			clip.MaxDZ = max(clip.MaxDZ, clipTile.DZ)
		}
		clip.Tiles = append(clip.Tiles, clipTile)
	}
	if len(clip.Tiles) == 0 {
		return nil
	}
	clip.TotalTiles = len(clip.Tiles)
	return clip
}

func (e *Editor) Cut() *Clip {
	clip := e.Copy()
	if clip != nil {
		e.deleteSelection("Cut", true)
	}
	return clip
}

// CutAt copies the selection relative to anchorZ and removes the selected contents in one undo step.
func (e *Editor) CutAt(anchorZ int) *Clip {
	clip := e.CopyAt(anchorZ)
	if clip != nil {
		e.deleteSelection("Cut", true)
	}
	return clip
}

func validatePasteBounds(clip *Clip, at otmap.Position) error {
	if int(at.Z) > otmap.MaxFloor {
		return fmt.Errorf("cannot paste at floor %d: valid floors are 0..%d", at.Z, otmap.MaxFloor)
	}
	for _, clipTile := range clip.Tiles {
		x, y, z := int(at.X)+clipTile.DX, int(at.Y)+clipTile.DY, int(at.Z)+clipTile.DZ
		if z < 0 || z > otmap.MaxFloor {
			return fmt.Errorf("cannot paste here: tiles would fall outside valid floors 0..%d (got floor %d)", otmap.MaxFloor, z)
		}
		if x < 0 || y < 0 || x > 0xFFFF || y > 0xFFFF {
			return fmt.Errorf("cannot paste here: tiles would fall outside valid map coordinates 0..65535 (got %d, %d)", x, y)
		}
	}
	return nil
}

func (e *Editor) Paste(clip *Clip, at otmap.Position) error {
	_, err := e.paste(clip, at, false)
	return err
}

// PasteCrossMap pastes clip into another map, stripping foreign house IDs so
// the target map never receives orphaned or colliding house references.
func (e *Editor) PasteCrossMap(clip *Clip, at otmap.Position) (strippedHouses bool, err error) {
	return e.paste(clip, at, true)
}

func (e *Editor) paste(clip *Clip, at otmap.Position, stripHouses bool) (bool, error) {
	if clip == nil || len(clip.Tiles) == 0 {
		return false, errors.New("the clipboard is empty")
	}
	if err := validatePasteBounds(clip, at); err != nil {
		return false, err
	}
	tx := e.BeginCap("Paste", 0, len(clip.Tiles)+len(e.selectedPositions))
	e.deselectAll(tx)
	trackAuto := e.Settings.Automagic
	var pasted, changed *posSet
	if trackAuto {
		pasted = newPosSetCap(len(clip.Tiles))
		changed = newPosSetCap(len(clip.Tiles))
	}
	strippedHouses := false
	for _, clipTile := range clip.Tiles {
		target, _ := at.Add(clipTile.DX, clipTile.DY, clipTile.DZ)
		if stripHouses && clipTile.HouseID != 0 {
			clipTile.HouseID = 0
			strippedHouses = true
		}
		e.put(tx.Edit(target), clipTile, true)
		if trackAuto {
			pasted.add(target)
			if e.reshapes(clipTile) {
				changed.add(target)
			}
		}
	}
	if trackAuto {
		e.autoAroundPaste(tx, pasted, changed)
	}
	tx.Commit()
	if len(e.selectedPositions) > 0 {
		e.selectionFloor = int(at.Z)
	} else {
		e.selectionFloor = -1
	}
	return strippedHouses, nil
}

func (e *Editor) neighborShapes(tile *otmap.Tile) (wall, table, carpet bool) {
	if tile == nil {
		return false, false, false
	}
	for _, item := range tile.Items {
		info := e.Brushes.info[item.ID]
		if info == nil {
			continue
		}
		wall = wall || info.wall != nil
		table = table || info.table != nil
		carpet = carpet || info.carpet != nil
	}
	return wall, table, carpet
}

// autoAroundPaste blends existing non-pasted destination neighbors around the
// pasted footprint without rewriting interior copied borders or walls.
func (e *Editor) autoAroundPaste(tx *Tx, pasted, changed *posSet) {
	if !e.Settings.Automagic || len(changed.list) == 0 || len(changed.list) > maxAutoTiles {
		return
	}
	outer := newPosSet()
	fitWallBoundary := newPosSet()
	fitTableBoundary := newPosSet()
	fitCarpetBoundary := newPosSet()
	for _, center := range changed.list {
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				if dx == 0 && dy == 0 {
					continue
				}
				neighbor, ok := center.Add(dx, dy, 0)
				if !ok || pasted.seen[neighbor] {
					continue
				}
				tile := tx.Get(neighbor)
				if tile == nil {
					continue
				}
				outer.add(neighbor)
				hasWall, hasTable, hasCarpet := e.neighborShapes(tile)
				if hasWall {
					fitWallBoundary.add(center)
				}
				if hasTable {
					fitTableBoundary.add(center)
				}
				if hasCarpet {
					fitCarpetBoundary.add(center)
				}
			}
		}
	}
	for _, pos := range outer.list {
		e.fitToNeighbors(tx, pos)
	}
	for _, pos := range fitWallBoundary.list {
		e.fitWalls(tx, pos)
	}
	for _, pos := range fitTableBoundary.list {
		e.fitTables(tx, pos)
	}
	for _, pos := range fitCarpetBoundary.list {
		e.fitCarpets(tx, pos)
	}
}

// reshapes reports whether placing or removing clipTile changes the
// borders, walls, tables or carpets around it. Other items leave them
// alone, so moving a barrel does not redo hand-placed borders.
func (e *Editor) reshapes(clipTile ClipTile) bool {
	if clipTile.Ground != nil {
		return true
	}
	for _, item := range clipTile.Items {
		if info := e.Brushes.info[item.ID]; info != nil && (info.wall != nil || info.table != nil || info.carpet != nil) {
			return true
		}
	}
	return false
}

func (e *Editor) autoAround(tx *Tx, changed *posSet) {
	if !e.Settings.Automagic || len(changed.list) == 0 || len(changed.list) > maxAutoTiles {
		return
	}
	for _, pos := range changed.ring().list {
		e.fitToNeighbors(tx, pos)
	}
}

func (e *Editor) DeleteSelection(label string) int {
	return e.deleteSelection(label, false)
}

func (e *Editor) deleteSelection(label string, clearMeta bool) int {
	if len(e.selectedPositions) == 0 {
		return 0
	}
	if label == "" {
		label = "Delete"
	}
	tx := e.Begin(label, 0)
	changed := newPosSet()
	for _, pos := range e.Selected() {
		tile := tx.Edit(pos)
		clipTile := extract(tile, true)
		if clearMeta && clipTile.Ground != nil {
			tile.HouseID, tile.Flags = 0, 0
		}
		if e.reshapes(clipTile) {
			changed.add(pos)
		}
	}
	e.autoAround(tx, changed)
	count := tx.Commit()
	if len(e.selectedPositions) == 0 {
		e.selectionFloor = -1
	}
	return count
}

func (e *Editor) MoveSelection(dx, dy, dz int) error {
	if len(e.selectedPositions) == 0 || (dx == 0 && dy == 0 && dz == 0) {
		return nil
	}
	selected := e.Selected()
	for _, pos := range selected {
		if _, ok := pos.Add(dx, dy, dz); !ok {
			return errors.New("the selection would leave the map")
		}
	}
	tx := e.Begin("Move", 0)
	type movedPart struct {
		to       otmap.Position
		clipTile ClipTile
	}
	var parts []movedPart
	changed := newPosSet()
	for _, from := range selected {
		clipTile := extract(tx.Edit(from), true)
		if clipTile.Ground != nil {
			// The house and flags move with the ground.
			tile := tx.Edit(from)
			tile.HouseID, tile.Flags = 0, 0
		}
		to, _ := from.Add(dx, dy, dz)
		parts = append(parts, movedPart{to, clipTile})
		if e.reshapes(clipTile) {
			changed.add(from)
		}
	}
	for _, part := range parts {
		e.put(tx.Edit(part.to), part.clipTile, true)
		if e.reshapes(part.clipTile) {
			changed.add(part.to)
		}
	}
	e.autoAround(tx, changed)
	tx.Commit()
	return nil
}

func (e *Editor) BorderizeSelection() int {
	return e.borderizePositions("Borderize selection", e.Selected())
}

func (e *Editor) RandomizeSelection() int {
	return e.randomizePositions("Randomize selection", e.Selected())
}

func (e *Editor) randomize(tx *Tx, pos otmap.Position) {
	current := tx.Get(pos)
	brush := e.groundBrushOf(current)
	if brush == nil || !brush.randomize {
		return
	}
	id := uint32(brush.items.pick(e.rng))
	if id == 0 || id == current.Ground.ID {
		return
	}
	tile := tx.Edit(pos)
	tile.Ground.ID = id
}
