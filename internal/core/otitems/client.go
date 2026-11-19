package otitems

import (
	"math"

	"github.com/nekiro/ots-creator/internal/core/thing"
)

type ThingSource interface {
	Thing(category thing.Category, id uint32) (*thing.Thing, error)
}

// FromClient builds a database where server ids are client ids, from every
// item of the client. maxID is the highest client item id (full 32-bit uint32).
func FromClient(source ThingSource, maxID uint32, names map[uint32]string) *DB {
	db := &DB{Mode: ModeClient}
	minID := thing.CategoryItem.MinID()
	if maxID < minID {
		db.index()
		return db
	}
	for id := minID; id <= maxID; id++ {
		clientThing, err := source.Thing(thing.CategoryItem, id)
		if err == nil && clientThing != nil {
			itemType := &Type{ID32: id, ClientID32: id, Name: names[id]}
			if id <= math.MaxUint16 {
				itemType.ID = uint16(id)
				itemType.ClientID = uint16(id)
			}
			applyThing(itemType, clientThing, true)
			db.set(itemType)
		}
		if id == maxID {
			break
		}
	}
	db.index()
	return db
}

// ApplyClient completes an otb database with what only the client stores
// (minimap colors, full ground, light) and fills flags of items whose otb
// entry has none.
func (db *DB) ApplyClient(source ThingSource) {
	db.Each(func(itemType *Type) {
		clientID := itemType.EffectiveClientID()
		if clientID == 0 {
			return
		}
		clientThing, err := source.Thing(thing.CategoryItem, clientID)
		if err != nil || clientThing == nil {
			return
		}
		applyThing(itemType, clientThing, false)
	})
}

// applyThing copies client flags. all also sets what items.otb normally
// provides (group, top order, blocking...).
func applyThing(itemType *Type, clientThing *thing.Thing, all bool) {
	applyClientOnly(itemType, clientThing)
	if all {
		applyServerFlags(itemType, &clientThing.Props)
		applyServerGroup(itemType, &clientThing.Props)
	} else {
		applyHookFlags(itemType, &clientThing.Props)
	}
}

// applyClientOnly copies what items.otb does not store: minimap color, full
// ground, light, corpse and (when missing) the name.
func applyClientOnly(itemType *Type, clientThing *thing.Thing) {
	props := &clientThing.Props
	if props.MiniMap && itemType.MinimapColor == 0 {
		itemType.MinimapColor = props.MiniMapColor
	}
	if props.FullGround {
		itemType.Flags |= FlagFullGround
	}
	if props.HasLight && itemType.LightLevel == 0 {
		itemType.LightLevel, itemType.LightColor = uint8(props.LightLevel), uint8(props.LightColor)
	}
	if props.HasLight {
		itemType.Flags |= FlagLight
	}
	if props.Corpse || props.PlayerCorpse {
		itemType.Flags |= FlagCorpse
	}
	if itemType.Name == "" && clientThing.Name != "" {
		itemType.Name = clientThing.Name
	}
}

// applyHookFlags adds the hook flags otb clients still need for drawing.
func applyHookFlags(itemType *Type, props *thing.Properties) {
	if props.HookSouth {
		itemType.Flags |= FlagHookSouth
	}
	if props.HookEast {
		itemType.Flags |= FlagHookEast
	}
	if props.Hangable {
		itemType.Flags |= FlagHangable
	}
}

func applyServerFlags(itemType *Type, props *thing.Properties) {
	set := func(on bool, flag Flags) {
		if on {
			itemType.Flags |= flag
		}
	}
	set(props.Ground, FlagGround)
	set(props.Unpassable, FlagUnpassable)
	set(props.BlockMissile, FlagBlockMissile)
	set(props.BlockPathfind, FlagBlockPath)
	set(props.Pickupable, FlagPickupable)
	set(!props.Unmoveable, FlagMoveable)
	set(props.Stackable, FlagStackable)
	set(props.FluidContainer, FlagFluidContainer)
	set(props.Fluid, FlagSplash)
	set(props.Hangable, FlagHangable)
	set(props.HookSouth, FlagHookSouth)
	set(props.HookEast, FlagHookEast)
	set(props.Rotatable, FlagRotatable)
	set(props.Writable || props.WritableOnce, FlagReadable)
	set(props.Writable, FlagWritable)
	set(props.Container, FlagContainer)
	set(props.FloorChange, FlagFloorChange)
	set(props.HasElevation, FlagElevation)
	set(props.HasCharges, FlagCharges)
	set(props.IgnoreLook, FlagIgnoreLook)
}

func applyServerGroup(itemType *Type, props *thing.Properties) {
	switch {
	case props.GroundBorder:
		itemType.TopOrder = 1
	case props.OnBottom:
		itemType.TopOrder = 2
	case props.OnTop:
		itemType.TopOrder = 3
	}
	switch {
	case props.Ground:
		itemType.Group = GroupGround
		itemType.Speed = props.GroundSpeed
	case props.Container:
		itemType.Group = GroupContainer
	case props.Fluid:
		itemType.Group = GroupSplash
	case props.FluidContainer:
		itemType.Group = GroupFluid
	case props.Writable || props.WritableOnce:
		itemType.Group = GroupWritable
	}
	if props.Container {
		itemType.Kind = KindContainer
	}
}
