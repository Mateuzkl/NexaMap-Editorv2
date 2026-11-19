// Package otitems is the item type database of the map editor. Maps store
// server ids: with an items.otb (TFS and older servers) they map to client
// ids through it; without one (servers that use client ids
// directly) both ids are the same. Flags come from the otb when present and
// from the client (dat or appearances) otherwise; names come from items.xml
// or the client.
package otitems

import (
	"math"
	"slices"
	"strconv"
	"strings"
)

type Mode string

const (
	ModeOTB    Mode = "otb"
	ModeClient Mode = "client"
)

// denseLimit separates dense O(1) slice storage (0..65535) from sparse 32-bit
// map storage (>= 65536), matching SparseItemMap in NexaMap-Editor.
const denseLimit = 65536

// DB is the item database. It is read-only after loading.
type DB struct {
	Mode         Mode   `json:"mode"`
	MajorVersion uint32 `json:"majorVersion"`
	MinorVersion uint32 `json:"minorVersion"`
	BuildNumber  uint32 `json:"buildNumber"`
	items        []*Type
	sparse       map[uint32]*Type
	sparseKeys   []uint32
	maxID        uint32
	byClient     map[uint16]uint16
	byClient32   map[uint32]uint32
}

func (db *DB) Get(id uint16) *Type {
	if db == nil || int(id) >= len(db.items) {
		return nil
	}
	return db.items[id]
}

func (db *DB) Get32(id uint32) *Type {
	if db == nil {
		return nil
	}
	if id < denseLimit {
		if int(id) >= len(db.items) {
			return nil
		}
		return db.items[id]
	}
	if db.sparse == nil {
		return nil
	}
	return db.sparse[id]
}

func (db *DB) MaxID() uint16 {
	if db == nil || db.maxID == 0 {
		return 0
	}
	if db.maxID > math.MaxUint16 {
		return math.MaxUint16
	}
	return uint16(db.maxID)
}

func (db *DB) MaxID32() uint32 {
	if db == nil {
		return 0
	}
	return db.maxID
}

// FromClient returns the first server id that shows a client id.
func (db *DB) FromClient(clientID uint16) (uint16, bool) {
	id, ok := db.byClient[clientID]
	return id, ok
}

// FromClient32 returns the first 32-bit server id that shows a 32-bit client id.
func (db *DB) FromClient32(clientID uint32) (uint32, bool) {
	id, ok := db.byClient32[clientID]
	return id, ok
}

func (db *DB) Each(fn func(*Type)) {
	if db == nil {
		return
	}
	for _, itemType := range db.items {
		if itemType != nil {
			fn(itemType)
		}
	}
	for _, key := range db.sparseKeys {
		if itemType := db.sparse[key]; itemType != nil {
			fn(itemType)
		}
	}
}

func (db *DB) Count() int {
	if db == nil {
		return 0
	}
	count := len(db.sparse)
	for _, itemType := range db.items {
		if itemType != nil {
			count++
		}
	}
	return count
}

func (db *DB) set(itemType *Type) {
	id := itemType.EffectiveID()
	itemType.ID32 = id
	if id <= math.MaxUint16 {
		itemType.ID = uint16(id)
	}
	cid := itemType.EffectiveClientID()
	itemType.ClientID32 = cid
	if cid <= math.MaxUint16 {
		itemType.ClientID = uint16(cid)
	}
	rot := itemType.EffectiveRotateTo()
	itemType.RotateTo32 = rot
	if rot <= math.MaxUint16 {
		itemType.RotateTo = uint16(rot)
	}
	if id > db.maxID {
		db.maxID = id
	}
	if id < denseLimit {
		for int(id) >= len(db.items) {
			db.items = append(db.items, nil)
		}
		db.items[id] = itemType
		return
	}
	if db.sparse == nil {
		db.sparse = map[uint32]*Type{}
	}
	if _, existed := db.sparse[id]; !existed {
		db.sparseKeys = append(db.sparseKeys, id)
	}
	db.sparse[id] = itemType
}

func (db *DB) index() {
	slices.Sort(db.sparseKeys)
	total := len(db.items) + len(db.sparse)
	db.byClient = make(map[uint16]uint16, len(db.items))
	db.byClient32 = make(map[uint32]uint32, total)
	db.Each(func(itemType *Type) {
		cid := itemType.EffectiveClientID()
		id := itemType.EffectiveID()
		if cid == 0 {
			return
		}
		if _, ok := db.byClient32[cid]; !ok {
			db.byClient32[cid] = id
		}
		if cid <= math.MaxUint16 && id <= math.MaxUint16 {
			if _, ok := db.byClient[uint16(cid)]; !ok {
				db.byClient[uint16(cid)] = uint16(id)
			}
		}
	})
}

func (db *DB) Search(query string, limit int) []uint16 {
	var out []uint16
	for _, id := range db.Search32(query, limit) {
		if id <= math.MaxUint16 {
			out = append(out, uint16(id))
		}
	}
	return out
}

func (db *DB) Search32(query string, limit int) []uint32 {
	query = strings.ToLower(strings.TrimSpace(query))
	var out []uint32
	if query == "" {
		return out
	}
	if id, err := strconv.ParseUint(query, 10, 32); err == nil {
		if itemType := db.Get32(uint32(id)); itemType != nil {
			out = append(out, itemType.EffectiveID())
		}
	}
	db.Each(func(itemType *Type) {
		if len(out) >= limit {
			return
		}
		id := itemType.EffectiveID()
		if strings.Contains(strings.ToLower(itemType.Name), query) && !slices.Contains(out, id) {
			out = append(out, id)
		}
	})
	return out
}

func NewDB(mode Mode, types []*Type) *DB {
	db := &DB{Mode: mode}
	for _, itemType := range types {
		db.set(itemType)
	}
	db.index()
	return db
}

// MarkMeta marks ids as editor-only items (never saved), creating types for
// ids the database does not have.
func (db *DB) MarkMeta(ids []uint16) {
	for _, id := range ids {
		db.MarkMeta32([]uint32{uint32(id)})
	}
}

func (db *DB) MarkMeta32(ids []uint32) {
	for _, id := range ids {
		itemType := db.Get32(id)
		if itemType == nil {
			itemType = &Type{ID32: id, Name: "editor item"}
			db.set(itemType)
		}
		itemType.Flags |= FlagMeta
	}
}
