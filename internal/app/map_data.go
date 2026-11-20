package app

// Item data of the map editor.
//
// A map stores its items by id, in one of two ways (its item id mode,
// otitems.Mode):
//
//   - server ids (otitems.ModeOTB): the ids of the server, translated to the
//     sprites of the client with the items.otb attached to the client
//     (Session.ItemsOTB, the same file compiling keeps in sync);
//   - client ids (otitems.ModeClient): the ids of the client objects, used as
//     they are, without an items.otb.
//
// The mode belongs to each open map (opened.editor.Items.Mode). It is chosen
// when the map is opened or created (map_item_ids.go) and remembered for the
// map file. The service loads the item data of the open client once per mode
// (ms.data) and every map uses the data of its mode: dataFor.
//
// The server data of the client (items.xml, the brush file and the server
// folder, MapDataConfig) describe the server the client belongs to, so they
// apply to the server's mode: server ids when the client has an items.otb,
// else client ids. Data of the other mode (a map converted to client ids, or
// opened on client ids although the client has an items.otb) takes the names
// of the client and the default brush file of its mode.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/nekiro/ots-creator/internal/core/otitems"
	"github.com/nekiro/ots-creator/internal/core/project"
	"github.com/nekiro/ots-creator/internal/core/thing"
	"github.com/nekiro/ots-creator/internal/fsutil"
	"github.com/nekiro/ots-creator/internal/mapping/mapedit"
	"github.com/nekiro/ots-creator/internal/mapping/materials"
)

var errNoItemsOTB = errors.New("maps on server item ids need the items.otb of the server: attach it to the client")

type mapData struct {
	project *project.Project
	client  string
	config  MapDataConfig
	// otbPath is the items.otb the server ids were read with ("" for client
	// ids).
	otbPath       string
	items         *otitems.DB
	materials     *materials.Materials
	effective     *materials.Materials
	materialsPath string
	brushes       *mapedit.Brushes
	// creatures holds every known creature by lower case name; the
	// materials file wins over the creatures scanned from the server.
	creatures map[string]*materials.CreatureType
	scanned   []*materials.CreatureType
	warnings  []string
}

type projectSource struct{ items []*thing.Thing }

func (source projectSource) Thing(_ thing.Category, id uint32) (*thing.Thing, error) {
	minID := thing.CategoryItem.MinID()
	if id >= minID && int(id-minID) < len(source.items) {
		if found := source.items[id-minID]; found != nil {
			return found, nil
		}
	}
	return nil, errors.New("missing")
}

func clientItems(proj *project.Project) projectSource {
	return projectSource{items: proj.ItemsSnapshot()}
}

func clientKey(proj *project.Project) string {
	info := proj.Info()
	if info.DatPath == "" {
		return "unsaved client " + info.Version.Name
	}
	return info.DatPath
}

// serverMode is the item id mode of the server the client belongs to:
// server ids when the client has an items.otb attached. New maps and the
// palette without an open map use it.
func (ms *MapService) serverMode() otitems.Mode {
	if ms.session.ItemsOTB() != "" {
		return otitems.ModeOTB
	}
	return otitems.ModeClient
}

// current returns the loaded item data of mode while it fits the open
// client and its items.otb, else nil. Server ids keep their data when the
// items.otb is detached: the maps on them stay readable.
func (ms *MapService) current(mode otitems.Mode) *mapData {
	data := ms.data[mode]
	proj, err := ms.session.Project()
	if data == nil || err != nil || data.project != proj {
		return nil
	}
	if attached := ms.session.ItemsOTB(); mode == otitems.ModeOTB && attached != "" && !fsutil.SamePath(attached, data.otbPath) {
		return nil
	}
	return data
}

// dataOf returns the item data of mode, loading it the first time and
// after the client or its items.otb changed. The caller holds the write
// lock.
func (ms *MapService) dataOf(mode otitems.Mode) (*mapData, error) {
	if data := ms.current(mode); data != nil {
		return data, nil
	}
	proj, err := ms.session.Project()
	if err != nil {
		return nil, errNoClient
	}
	data, err := ms.loadData(proj, mode)
	if err != nil {
		return nil, err
	}
	ms.setData(data)
	return data, nil
}

// dataFor returns the item data of the mode of an open map.
func (ms *MapService) dataFor(opened *openMap) (*mapData, error) {
	return ms.dataOf(opened.editor.Items.Mode)
}

func (ms *MapService) loadData(proj *project.Project, mode otitems.Mode) (*mapData, error) {
	client := clientKey(proj)
	config, _ := ms.store.Profile(client)
	data := &mapData{project: proj, client: client}
	// The server data of the client apply to the server's mode only.
	if mode == ms.serverMode() {
		data.config = config
	} else {
		data.config = MapDataConfig{ServerDir: config.ServerDir}
	}
	switch mode {
	case otitems.ModeOTB:
		data.otbPath = ms.session.ItemsOTB()
		if data.otbPath == "" {
			return nil, errNoItemsOTB
		}
	case otitems.ModeClient:
	default:
		return nil, fmt.Errorf("unknown item ids %q: use %q (server ids) or %q", mode, otitems.ModeOTB, otitems.ModeClient)
	}
	if err := data.loadItems(mode); err != nil {
		return nil, err
	}
	data.loadMaterials(ms.store.Dir())
	if dir := data.config.ServerDir; dir != "" {
		if creatures, err := materials.ScanCreatures(dir); err == nil {
			data.scanned = creatures
		}
	}
	data.index()
	return data, nil
}

// loadItems reads the item database: server ids from the items.otb, client
// ids from the client; then names from items.xml.
func (data *mapData) loadItems(mode otitems.Mode) error {
	source := clientItems(data.project)
	if mode == otitems.ModeOTB {
		otb, err := os.ReadFile(data.otbPath)
		if err != nil {
			return fmt.Errorf("items.otb: %w", err)
		}
		if data.items, err = otitems.ParseOTB(otb); err != nil {
			return fmt.Errorf("items.otb %s: %w", data.otbPath, err)
		}
		data.items.ApplyClient(source)
	} else {
		data.items = otitems.FromClient(source, data.project.Info().Counts.Items, data.project.Names(thing.CategoryItem))
	}
	if xml, err := fsutil.ReadOptional(data.config.XMLPath); err != nil {
		data.warnings = append(data.warnings, "items.xml: "+err.Error())
	} else if xml != nil {
		if err := data.items.ApplyXML(xml); err != nil {
			data.warnings = append(data.warnings, err.Error())
		}
	}
	return nil
}

// loadMaterials reads the brush file: the configured one, else one per
// client version and item id mode in storeDir.
func (data *mapData) loadMaterials(storeDir string) {
	data.materialsPath = data.config.MaterialsPath
	if data.materialsPath == "" && storeDir != "" {
		name := fmt.Sprintf("%d-%s.json", data.project.Info().Version.Value, data.items.Mode)
		data.materialsPath = filepath.Join(storeDir, "materials", name)
	}
	data.materials = materials.New()
	if data.materialsPath == "" {
		return
	}
	if loaded, err := materials.Load(data.materialsPath); err == nil {
		data.materials = loaded
	} else if !os.IsNotExist(err) {
		data.warnings = append(data.warnings, err.Error())
	}
}

func (data *mapData) hasItem(id uint16) bool {
	if id == 0 || data.items == nil {
		return false
	}
	it := data.items.Get(id)
	return it != nil && it.EffectiveClientID() != 0
}

func (data *mapData) resolveBrushLook(brush *materials.Brush, fallback uint16) uint16 {
	if data.hasItem(fallback) {
		return fallback
	}
	if data.hasItem(brush.Look) {
		return brush.Look
	}
	for _, ch := range brush.Items {
		if data.hasItem(ch.ID) {
			return ch.ID
		}
	}
	for _, align := range materials.WallAlignments {
		if part := brush.Walls[align]; part != nil {
			for _, ch := range part.Items {
				if data.hasItem(ch.ID) {
					return ch.ID
				}
			}
		}
	}
	for _, chances := range brush.Parts {
		for _, ch := range chances {
			if data.hasItem(ch.ID) {
				return ch.ID
			}
		}
	}
	for _, alt := range brush.Alternatives {
		for _, single := range alt.Singles {
			if data.hasItem(single.Item.ID) {
				return single.Item.ID
			}
		}
		for _, comp := range alt.Composites {
			for _, tile := range comp.Tiles {
				for _, item := range tile.Items {
					if data.hasItem(item.ID) {
						return item.ID
					}
				}
			}
		}
	}
	return 0
}

func (data *mapData) filterBuiltinBorder(b *materials.Border, assignedID uint32) *materials.Border {
	if b == nil {
		return nil
	}
	edges := make(map[string]uint16, len(b.Edges))
	for edge, id := range b.Edges {
		if data.hasItem(id) {
			edges[edge] = id
		}
	}
	if len(edges) == 0 {
		return nil
	}
	groundEq := b.GroundEquivalent
	if groundEq != 0 && !data.hasItem(groundEq) {
		groundEq = 0
	}
	return &materials.Border{
		ID:               assignedID,
		Group:            b.Group,
		Edges:            edges,
		GroundEquivalent: groundEq,
		Optional:         b.Optional,
	}
}

func (data *mapData) filterBorderRule(rule materials.BorderRule, borderMap map[uint32]uint32) (materials.BorderRule, bool) {
	out := rule
	if out.Border != 0 {
		mapped, ok := borderMap[out.Border]
		if !ok {
			return materials.BorderRule{}, false
		}
		out.Border = mapped
	}
	if out.Inline != nil {
		filteredInline := data.filterBuiltinBorder(out.Inline, 0)
		if filteredInline == nil {
			return materials.BorderRule{}, false
		}
		out.Inline = filteredInline
	}
	if len(out.Specific) > 0 {
		var filteredCases []materials.SpecificCase
		for _, sc := range out.Specific {
			ok := true
			conds := make([]materials.Condition, 0, len(sc.Conditions))
			for _, c := range sc.Conditions {
				if c.Border != 0 {
					mapped, exists := borderMap[c.Border]
					if !exists {
						ok = false
						break
					}
					c.Border = mapped
				}
				if c.Item != 0 && !data.hasItem(c.Item) {
					ok = false
					break
				}
				conds = append(conds, c)
			}
			if !ok {
				continue
			}
			acts := make([]materials.Action, 0, len(sc.Actions))
			for _, a := range sc.Actions {
				if a.Border != 0 {
					mapped, exists := borderMap[a.Border]
					if !exists {
						ok = false
						break
					}
					a.Border = mapped
				}
				if (a.Item != 0 && !data.hasItem(a.Item)) || (a.With != 0 && !data.hasItem(a.With)) {
					ok = false
					break
				}
				acts = append(acts, a)
			}
			if ok && len(conds) > 0 && len(acts) > 0 {
				filteredCases = append(filteredCases, materials.SpecificCase{Conditions: conds, Actions: acts})
			}
		}
		out.Specific = filteredCases
	}
	return out, out.Border != 0 || out.Inline != nil
}

func (data *mapData) filterBuiltinBrush(b *materials.Brush, borderMap map[uint32]uint32) *materials.Brush {
	if b == nil {
		return nil
	}
	out := *b
	out.Items = nil
	out.Borders = nil
	out.Optional = nil
	out.Walls = nil
	out.Parts = nil
	out.Alternatives = nil

	for _, ch := range b.Items {
		if data.hasItem(ch.ID) {
			out.Items = append(out.Items, ch)
		}
	}
	if b.Optional != nil {
		if rule, ok := data.filterBorderRule(*b.Optional, borderMap); ok {
			out.Optional = &rule
		}
	}
	for _, br := range b.Borders {
		if rule, ok := data.filterBorderRule(br, borderMap); ok {
			out.Borders = append(out.Borders, rule)
		}
	}
	if len(b.Walls) > 0 {
		walls := make(map[string]*materials.WallPart, len(b.Walls))
		for align, part := range b.Walls {
			if part == nil {
				continue
			}
			fp := &materials.WallPart{}
			for _, ch := range part.Items {
				if data.hasItem(ch.ID) {
					fp.Items = append(fp.Items, ch)
				}
			}
			for _, door := range part.Doors {
				if data.hasItem(door.ID) {
					fp.Doors = append(fp.Doors, door)
				}
			}
			if len(fp.Items) > 0 || len(fp.Doors) > 0 {
				walls[align] = fp
			}
		}
		if len(walls) > 0 {
			out.Walls = walls
		}
	}
	if len(b.Parts) > 0 {
		parts := make(map[string][]materials.Chance, len(b.Parts))
		for align, list := range b.Parts {
			var filtered []materials.Chance
			for _, ch := range list {
				if data.hasItem(ch.ID) {
					filtered = append(filtered, ch)
				}
			}
			if len(filtered) > 0 {
				parts[align] = filtered
			}
		}
		if len(parts) > 0 {
			out.Parts = parts
		}
	}
	for _, alt := range b.Alternatives {
		var fa materials.Alternative
		for _, single := range alt.Singles {
			if data.hasItem(single.Item.ID) {
				fa.Singles = append(fa.Singles, single)
			}
		}
		for _, comp := range alt.Composites {
			allValid := len(comp.Tiles) > 0
			for _, tile := range comp.Tiles {
				for _, item := range tile.Items {
					if !data.hasItem(item.ID) {
						allValid = false
						break
					}
				}
				if !allValid {
					break
				}
			}
			if allValid {
				fa.Composites = append(fa.Composites, comp)
			}
		}
		if len(fa.Singles) > 0 || len(fa.Composites) > 0 {
			out.Alternatives = append(out.Alternatives, fa)
		}
	}

	out.Look = data.resolveBrushLook(&out, b.Look)
	if out.Look == 0 {
		return nil
	}
	return &out
}

func (data *mapData) buildEffectiveMaterials() *materials.Materials {
	user := data.materials
	if user == nil {
		user = materials.New()
	}
	var mode otitems.Mode = otitems.ModeOTB
	if data.items != nil {
		mode = data.items.Mode
	}
	builtin, err := materials.BuiltinMaterials(mode)
	if err != nil {
		msg := "built-in brushes: " + err.Error()
		if !slices.Contains(data.warnings, msg) {
			data.warnings = append(data.warnings, msg)
		}
	}

	// Skip the bundled Tibia/RME catalog only on tiny synthetic projects
	// (e.g. minimal 3-item test fixtures), never on real OT/Tibia clients.
	useBuiltin := builtin != nil && data.items != nil && data.items.Count() >= 16

	eff := &materials.Materials{
		Version:   materials.FormatVersion,
		Borders:   make([]*materials.Border, 0, len(user.Borders)+len(builtin.Borders)),
		Brushes:   make([]*materials.Brush, 0, len(user.Brushes)+len(builtin.Brushes)),
		Tilesets:  make([]*materials.Tileset, 0, len(user.Tilesets)+len(builtin.Tilesets)),
		Creatures: user.Creatures,
		MetaItems: make([]uint16, 0, len(user.MetaItems)+len(builtin.MetaItems)),
	}

	userBorders := make(map[uint32]bool, len(user.Borders))
	var maxBorderID uint32
	for _, b := range user.Borders {
		eff.Borders = append(eff.Borders, b)
		userBorders[b.ID] = true
		if b.ID > maxBorderID {
			maxBorderID = b.ID
		}
	}

	borderMap := map[uint32]uint32{}
	if useBuiltin {
		for _, b := range builtin.Borders {
			if b.ID > maxBorderID {
				maxBorderID = b.ID
			}
		}
		nextBorderID := maxBorderID + 1
		for _, b := range builtin.Borders {
			assignedID := b.ID
			if userBorders[assignedID] {
				assignedID = nextBorderID
				nextBorderID++
			}
			filtered := data.filterBuiltinBorder(b, assignedID)
			if filtered == nil {
				continue
			}
			eff.Borders = append(eff.Borders, filtered)
			borderMap[b.ID] = assignedID
		}
	}

	userBrushes := make(map[string]bool, len(user.Brushes))
	for _, b := range user.Brushes {
		eff.Brushes = append(eff.Brushes, b)
		userBrushes[strings.ToLower(b.Name)] = true
	}
	if useBuiltin {
		for _, b := range builtin.Brushes {
			if userBrushes[strings.ToLower(b.Name)] {
				continue
			}
			if filtered := data.filterBuiltinBrush(b, borderMap); filtered != nil {
				eff.Brushes = append(eff.Brushes, filtered)
			}
		}
	}

	userTilesets := make(map[string]bool, len(user.Tilesets))
	for _, ts := range user.Tilesets {
		eff.Tilesets = append(eff.Tilesets, ts)
		userTilesets[strings.ToLower(ts.Name)] = true
	}
	if useBuiltin {
		for _, ts := range builtin.Tilesets {
			if !userTilesets[strings.ToLower(ts.Name)] {
				eff.Tilesets = append(eff.Tilesets, ts)
			}
		}
	}

	seenMeta := make(map[uint16]bool, len(user.MetaItems)+len(builtin.MetaItems))
	for _, id := range user.MetaItems {
		if !seenMeta[id] {
			seenMeta[id] = true
			eff.MetaItems = append(eff.MetaItems, id)
		}
	}
	if useBuiltin {
		for _, id := range builtin.MetaItems {
			if !seenMeta[id] && data.items.Get(id) != nil {
				seenMeta[id] = true
				eff.MetaItems = append(eff.MetaItems, id)
			}
		}
	}
	return eff
}

func (data *mapData) index() {
	data.effective = data.buildEffectiveMaterials()
	data.brushes = mapedit.NewBrushes(data.items, data.effective)
	data.creatures = map[string]*materials.CreatureType{}
	for _, creature := range data.allCreatures() {
		data.creatures[strings.ToLower(creature.Name)] = creature
	}
}

func (data *mapData) allCreatures() []*materials.CreatureType {
	var all []*materials.CreatureType
	defined := map[string]bool{}
	add := func(creature *materials.CreatureType) {
		key := strings.ToLower(creature.Name)
		if key == "" || defined[key] {
			return
		}
		defined[key] = true
		all = append(all, creature)
	}
	for _, creature := range data.materials.Creatures {
		add(creature)
	}
	for _, creature := range data.scanned {
		add(creature)
	}
	if len(all) == 0 {
		for _, creature := range materials.BuiltinCreatures() {
			add(creature)
		}
	}
	slices.SortFunc(all, func(first, second *materials.CreatureType) int {
		return strings.Compare(strings.ToLower(first.Name), strings.ToLower(second.Name))
	})
	return all
}

func (data *mapData) outfit(creature string) (materials.Outfit, bool) {
	if found := data.creatures[strings.ToLower(creature)]; found != nil {
		return found.Outfit, true
	}
	if found := materials.BuiltinCreature(creature); found != nil {
		return found.Outfit, true
	}
	return materials.Outfit{}, false
}

// setData keeps data for its mode and gives it to the open maps of that
// mode.
func (ms *MapService) setData(data *mapData) {
	mode := data.items.Mode
	ms.data[mode] = data
	for _, opened := range ms.maps {
		if opened.editor.Items.Mode == mode {
			opened.editor.Items = data.items
			opened.editor.SetBrushes(data.brushes)
		}
	}
}

// activeMode is the item id mode of the map in the active tab, else the
// server's.
func (ms *MapService) activeMode() otitems.Mode {
	if opened := ms.maps[ms.activeID]; opened != nil {
		return opened.editor.Items.Mode
	}
	return ms.serverMode()
}

// activeData returns the item data of activeMode(); when no map is open and
// loading server ids (items.otb) fails, it falls back to client ids so the
// map editor and library remain usable.
func (ms *MapService) activeData() (*mapData, error) {
	mode := ms.activeMode()
	data, err := ms.dataOf(mode)
	if err != nil && err != errNoClient && mode == otitems.ModeOTB && ms.maps[ms.activeID] == nil {
		if fallback, fbErr := ms.dataOf(otitems.ModeClient); fbErr == nil {
			return fallback, nil
		}
	}
	return data, err
}

// reloadData reads the item data of every mode in use again (after the
// client objects or the server data changed) and returns the active one.
func (ms *MapService) reloadData() (DataInfo, error) {
	proj, err := ms.session.Project()
	if err != nil {
		return DataInfo{}, errNoClient
	}
	active := ms.activeMode()
	modes := []otitems.Mode{active}
	for mode, data := range ms.data {
		if mode != active && data.project == proj {
			modes = append(modes, mode)
		}
	}
	var result DataInfo
	for _, mode := range modes {
		data, err := ms.loadData(proj, mode)
		if err != nil {
			if mode == active {
				return DataInfo{}, err
			}
			// A map on server ids keeps its data when the items.otb is gone.
			continue
		}
		ms.setData(data)
		if mode == active {
			result = data.info()
		}
	}
	return result, nil
}

type DataInfo struct {
	Ready  bool          `json:"ready"`
	Error  string        `json:"error,omitempty"`
	Client string        `json:"client"`
	Config MapDataConfig `json:"config"`
	// Mode is the item id mode the data is for.
	Mode otitems.Mode `json:"mode"`
	// OTBPath is the items.otb the server ids are read with ("" for client
	// ids), OTBVersion its version.
	OTBPath    string   `json:"otbPath"`
	OTBVersion string   `json:"otbVersion"`
	Items      int      `json:"items"`
	Materials  string   `json:"materials"`
	Brushes    int      `json:"brushes"`
	Tilesets   int      `json:"tilesets"`
	Creatures  int      `json:"creatures"`
	Warnings   []string `json:"warnings"`
}

func (data *mapData) info() DataInfo {
	info := DataInfo{
		Ready: true, Client: data.client, Config: data.config, Mode: data.items.Mode, Items: data.items.Count(),
		OTBPath: data.otbPath, Materials: data.materialsPath, Brushes: len(data.materials.Brushes),
		Tilesets: len(data.materials.Tilesets), Creatures: len(data.creatures),
		Warnings: slices.Concat(data.warnings, data.brushes.Warnings),
	}
	if data.items.Mode == otitems.ModeOTB {
		info.OTBVersion = fmt.Sprintf("%d.%d (build %d)", data.items.MajorVersion, data.items.MinorVersion, data.items.BuildNumber)
	}
	return info
}

type MapStatus struct {
	// Data is the item data of the active map, else of the server's mode.
	Data     DataInfo            `json:"data"`
	Maps     []MapInfo           `json:"maps"`
	Recent   []RecentMap         `json:"recent"`
	Settings mapedit.Settings    `json:"settings"`
	Keys     map[string][]string `json:"keys"`
}

func (ms *MapService) Status() MapStatus {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	status := MapStatus{Recent: ms.store.Recent(), Settings: ms.store.EditorSettings(), Keys: ms.store.Keys()}
	if data, err := ms.activeData(); err == nil {
		status.Data = data.info()
	} else if err != errNoClient {
		status.Data.Error = err.Error()
	}
	ids := make([]int, 0, len(ms.maps))
	for id := range ms.maps {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		status.Maps = append(status.Maps, ms.info(ms.maps[id], true))
	}
	return status
}

// DetectData looks for the server data (items.otb, items.xml and the data
// folder) in a server folder, a map folder or their parents.
func (ms *MapService) DetectData(path string) ServerData { return detectServerData(path) }

// SetData stores the server data of the open client and reads the item
// data again.
func (ms *MapService) SetData(config MapDataConfig) (DataInfo, error) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	proj, err := ms.session.Project()
	if err != nil {
		return DataInfo{}, errNoClient
	}
	client := clientKey(proj)
	stored, _ := ms.store.Profile(client)
	config.LegacyOTBPath = stored.LegacyOTBPath
	saveErr := ms.store.SetProfile(client, config)
	info, err := ms.reloadData()
	if err != nil {
		return DataInfo{}, err
	}
	if saveErr != nil {
		info.Warnings = append(info.Warnings, "settings: "+saveErr.Error())
	}
	return info, nil
}

// Reload reads the item data again, after the client objects or the
// items.otb changed.
func (ms *MapService) Reload() (DataInfo, error) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	return ms.reloadData()
}

type EditorPrefs struct {
	Settings mapedit.Settings    `json:"settings"`
	Keys     map[string][]string `json:"keys"`
}

// EditorPrefs gives the map editor settings and keys without loading the
// server data, for the settings dialog of the object editor.
func (ms *MapService) EditorPrefs() EditorPrefs {
	return EditorPrefs{Settings: ms.store.EditorSettings(), Keys: ms.store.Keys()}
}

func (ms *MapService) SetEditorSettings(settings mapedit.Settings) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	for _, opened := range ms.maps {
		opened.editor.Settings = settings
	}
	return ms.store.SetEditorSettings(settings)
}

// SetKeys stores the keyboard shortcuts changed by the user (action id ->
// key combinations); nil restores the defaults.
func (ms *MapService) SetKeys(keys map[string][]string) error {
	return ms.store.SetKeys(keys)
}
