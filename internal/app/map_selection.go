package app

import (
	"errors"
	"math"

	"github.com/nekiro/ots-creator/internal/mapping/mapedit"
	"github.com/nekiro/ots-creator/internal/mapping/otmap"
)

var (
	errNothingSelected       = errors.New("nothing is selected")
	errClipboardModeMismatch = errors.New("the clipboard uses a different item ID mode; copy items from a map with the same mode")
)

// selectionEdit is edit for selection changes: Count is the size of the
// selection afterwards.
func (ms *MapService) selectionEdit(id int, fn func(editor *mapedit.Editor)) (MapChange, error) {
	return ms.edit(id, func(editor *mapedit.Editor, _ *mapData) (int, error) {
		fn(editor)
		return editor.SelectionCount(), nil
	})
}

func (ms *MapService) SelectRect(id int, rect mapedit.Rect, floors string, add, remove, compensate bool) (MapChange, error) {
	return ms.selectionEdit(id, func(editor *mapedit.Editor) { editor.SelectRect(rect, floors, add, remove, compensate) })
}

func (ms *MapService) SelectAt(id int, pos otmap.Position, toggle, creatures, spawns bool) (MapChange, error) {
	return ms.selectionEdit(id, func(editor *mapedit.Editor) { editor.SelectAt(pos, toggle, creatures, spawns) })
}

func (ms *MapService) SelectItem(id int, pos otmap.Position, index int, add bool) (MapChange, error) {
	return ms.selectionEdit(id, func(editor *mapedit.Editor) { editor.SelectItem(pos, index, add) })
}

func (ms *MapService) SelectTiles(id int, positions []otmap.Position, add bool) (MapChange, error) {
	return ms.selectionEdit(id, func(editor *mapedit.Editor) { editor.SelectTiles(positions, add) })
}

func (ms *MapService) ClearSelection(id int) (MapChange, error) {
	return ms.editMap(id, func(editor *mapedit.Editor) error { editor.ClearSelection(); return nil })
}

func (ms *MapService) SelectAll(id int, floor int) (MapChange, error) {
	z := uint8(floor)
	whole := mapedit.Rect{From: otmap.Position{Z: z}, To: otmap.Position{X: math.MaxUint16, Y: math.MaxUint16, Z: z}}
	return ms.selectionEdit(id, func(editor *mapedit.Editor) {
		editor.SelectRect(whole, mapedit.FloorsCurrent, false, false, false)
	})
}

// Copy copies the selection to the clipboard (shared by all maps) and
// returns the number of copied tiles.
func (ms *MapService) Copy(id int) (tiles int, err error) {
	return ms.copySelection(id, -1)
}

// CopyAt copies the selection anchored to floor (DZ = tile.Z - floor) so
// multi-floor selections paste back onto the expected vertical floors.
func (ms *MapService) CopyAt(id int, floor int) (tiles int, err error) {
	return ms.copySelection(id, floor)
}

func (ms *MapService) clearClipboardLocked() {
	ms.clipboard = nil
	ms.clipboardMode = ""
	ms.clipboardMapID = 0
	ms.clipboardClient = ""
}

// dropStaleClipboardLocked releases the clipboard if the open client project
// changed since the copy was taken, preventing stale memory retention and
// cross-client ID corruption.
func (ms *MapService) dropStaleClipboardLocked() {
	if ms.clipboard == nil || ms.clipboardClient == "" {
		return
	}
	proj, err := ms.session.Project()
	if err != nil || clientKey(proj) != ms.clipboardClient {
		ms.clearClipboardLocked()
	}
}

func (ms *MapService) copySelection(id int, floor int) (tiles int, err error) {
	err = ms.withMap(id, func(opened *openMap, data *mapData) error {
		var clip *mapedit.Clip
		if floor >= 0 {
			clip = opened.editor.CopyAt(floor)
		} else {
			clip = opened.editor.Copy()
		}
		if clip == nil {
			return errNothingSelected
		}
		ms.clipboard = clip
		ms.clipboardMode = opened.editor.Items.Mode
		ms.clipboardMapID = id
		ms.clipboardClient = data.client
		tiles = len(clip.Tiles)
		return nil
	})
	return tiles, err
}

func (ms *MapService) Cut(id int) (MapChange, error) {
	return ms.cutSelection(id, -1)
}

// CutAt copies the selection anchored to floor and deletes it in one undo step.
func (ms *MapService) CutAt(id int, floor int) (MapChange, error) {
	return ms.cutSelection(id, floor)
}

func (ms *MapService) cutSelection(id int, floor int) (MapChange, error) {
	return ms.edit(id, func(editor *mapedit.Editor, data *mapData) (int, error) {
		var clip *mapedit.Clip
		if floor >= 0 {
			clip = editor.CutAt(floor)
		} else {
			clip = editor.Cut()
		}
		if clip == nil {
			return 0, errNothingSelected
		}
		ms.clipboard = clip
		ms.clipboardMode = editor.Items.Mode
		ms.clipboardMapID = id
		ms.clipboardClient = data.client
		return len(clip.Tiles), nil
	})
}

func (ms *MapService) Clipboard() *mapedit.Clip {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.dropStaleClipboardLocked()
	if opened := ms.maps[ms.activeID]; opened != nil && ms.clipboardMode != opened.editor.Items.Mode {
		return nil
	}
	return ms.clipboard
}

// ClipboardFor returns the clipboard for map id, reporting an error if the
// clipboard was copied from a map with an incompatible item ID mode.
func (ms *MapService) ClipboardFor(id int) (*mapedit.Clip, error) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.dropStaleClipboardLocked()
	if ms.clipboard == nil || len(ms.clipboard.Tiles) == 0 {
		return nil, nil
	}
	if opened := ms.maps[id]; opened != nil && ms.clipboardMode != opened.editor.Items.Mode {
		return nil, errClipboardModeMismatch
	}
	return ms.clipboard, nil
}

// ClipboardPreview returns clipboard metadata and at most maxTiles preview
// tiles for map id. When maxTiles <= 0 or the clipboard exceeds maxTiles,
// Tiles is omitted while TotalTiles and bounding offsets stay exact.
func (ms *MapService) ClipboardPreview(id int, maxTiles int) (*mapedit.Clip, error) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.dropStaleClipboardLocked()
	if ms.clipboard == nil || len(ms.clipboard.Tiles) == 0 {
		return nil, nil
	}
	if opened := ms.maps[id]; opened != nil && ms.clipboardMode != opened.editor.Items.Mode {
		return nil, errClipboardModeMismatch
	}
	return ms.clipboard.Preview(maxTiles), nil
}

// Paste puts the clipboard with its top-left corner at pos.
func (ms *MapService) Paste(id int, pos otmap.Position) (MapChange, error) {
	var strippedHouses bool
	change, err := ms.edit(id, func(editor *mapedit.Editor, _ *mapData) (int, error) {
		ms.dropStaleClipboardLocked()
		if ms.clipboard != nil && ms.clipboardMode != editor.Items.Mode {
			return 0, errClipboardModeMismatch
		}
		if ms.clipboardMapID != 0 && ms.clipboardMapID != id {
			var err error
			strippedHouses, err = editor.PasteCrossMap(ms.clipboard, pos)
			if err != nil {
				return 0, err
			}
			return len(ms.clipboard.Tiles), nil
		}
		if err := editor.Paste(ms.clipboard, pos); err != nil {
			return 0, err
		}
		return len(ms.clipboard.Tiles), nil
	})
	if err == nil && strippedHouses {
		change.Warnings = append(change.Warnings, "Cleared source map house IDs when pasting into a different map to avoid house ID conflicts.")
	}
	return change, err
}

func (ms *MapService) DeleteSelection(id int) (MapChange, error) {
	return ms.edit(id, func(editor *mapedit.Editor, _ *mapData) (int, error) { return editor.DeleteSelection(""), nil })
}

func (ms *MapService) MoveSelection(id int, dx, dy, dz int) (MapChange, error) {
	return ms.editMap(id, func(editor *mapedit.Editor) error { return editor.MoveSelection(dx, dy, dz) })
}
