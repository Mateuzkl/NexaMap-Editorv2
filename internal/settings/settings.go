package settings

import (
	"encoding/json"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/nekiro/ots-creator/internal/fsutil"

	"github.com/nekiro/ots-creator/internal/core/client"
)

const MaxRecent = 10
const MaxWorkspaces = 20

const (
	BackgroundMagenta     = "magenta"
	BackgroundTransparent = "transparent"
)

const (
	ThemeClassic     = "classic"
	ThemeModernDark  = "modern-dark"
	ThemeModernLight = "modern-light"
)

const (
	ItemIDsAuto   = "auto"
	ItemIDsClient = "client"
	ItemIDsServer = "server"
)

type WorkspaceProfile struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	ClientRoot         string `json:"clientRoot"`
	ServerRoot         string `json:"serverRoot"`
	SelectedClientPath string `json:"selectedClientPath,omitempty"`
	SelectedOtbPath    string `json:"selectedOtbPath,omitempty"`
	SelectedXmlPath    string `json:"selectedXmlPath,omitempty"`
	PreferredItemIds   string `json:"preferredItemIds,omitempty"`
	LastUsedAt         string `json:"lastUsedAt"`
}

type Recent struct {
	// Format is "assets" for a Tibia 12+ asset folder (DatPath is the
	// folder); empty for dat/spr.
	Format   string          `json:"format,omitempty"`
	DatPath  string          `json:"datPath"`
	SprPath  string          `json:"sprPath"`
	Version  client.Version  `json:"version"`
	Features client.Features `json:"features"`
	OpenedAt time.Time       `json:"openedAt"`
}

type Settings struct {
	CheckUpdates bool `json:"checkUpdates"`
	ReopenLast   bool `json:"reopenLast"`
	// SheetBackground fills empty pixels of exported sprite sheets
	// (BackgroundMagenta like ObjectBuilder, or BackgroundTransparent).
	SheetBackground string `json:"sheetBackground"`
	ExportFormat    string `json:"exportFormat"`
	// ObjectFormat is the file format of exported objects: "otobj" (the
	// native format) or "obd" (ObjectBuilder).
	ObjectFormat     string   `json:"objectFormat"`
	ListColumns      int      `json:"listColumns"`
	Recent           []Recent `json:"recent"`
	MarketAuthor     string   `json:"marketAuthor"`
	MarketAdminToken string   `json:"marketAdminToken"`
	// MarketShared maps market entries shared from this computer to their
	// delete tokens. Like Recent, Update keeps it as stored.
	MarketShared map[string]string `json:"marketShared"`
	// LastDirs maps a file dialog to the folder it last confirmed, so each
	// dialog opens where it was left. Like Recent, Update keeps it as stored.
	LastDirs map[string]string `json:"lastDirs"`
	// ItemsOTB maps a client (its dat path or asset folder) to the
	// items.otb kept in sync with it. Like Recent, Update keeps it as stored.
	ItemsOTB map[string]string `json:"itemsOtb"`
	// MCPServer runs the local MCP server through which AI agents edit
	// maps; MCPPort is its port on 127.0.0.1.
	MCPServer bool `json:"mcpServer"`
	MCPPort   int  `json:"mcpPort"`
	// UITheme selects the visual shell: "classic", "modern-dark" or "modern-light".
	UITheme          string             `json:"uiTheme"`
	UIScale          int                `json:"uiScale,omitempty"`
	SidebarCollapsed bool               `json:"sidebarCollapsed,omitempty"`
	WriteDebugLog    bool               `json:"writeDebugLog,omitempty"`
	WorkerThreads    int                `json:"workerThreads,omitempty"`
	UndoQueueSize    int                `json:"undoQueueSize,omitempty"`
	UndoMaxMemoryMB  int                `json:"undoMaxMemoryMb,omitempty"`
	Workspaces       []WorkspaceProfile `json:"workspaces"`
	ActiveWorkspace  string             `json:"activeWorkspaceId,omitempty"`
}

const (
	MinUIScale         = 80
	MaxUIScale         = 160
	DefaultUIScale     = 100
	DefaultUndoQueue   = 16384
	DefaultUndoMaxMB   = 4096
)

const (
	MinListColumns     = 6
	MaxListColumns     = 12
	DefaultListColumns = 7
)

const DefaultMCPPort = 47800

func Defaults() Settings {
	return Settings{
		CheckUpdates:    true,
		SheetBackground: BackgroundMagenta,
		ExportFormat:    "png",
		ObjectFormat:    "otobj",
		ListColumns:     DefaultListColumns,
		MCPPort:         DefaultMCPPort,
		UITheme:         ThemeModernDark,
		UIScale:         DefaultUIScale,
		WorkerThreads:   0,
		UndoQueueSize:   DefaultUndoQueue,
		UndoMaxMemoryMB: DefaultUndoMaxMB,
		Recent:          []Recent{},
		Workspaces:      []WorkspaceProfile{},
		MarketShared:    map[string]string{},
		LastDirs:        map[string]string{},
		ItemsOTB:        map[string]string{},
	}
}

func (s *Settings) normalize() {
	if s.SheetBackground != BackgroundTransparent {
		s.SheetBackground = BackgroundMagenta
	}
	switch s.ExportFormat {
	case "png", "bmp", "jpg":
	default:
		s.ExportFormat = "png"
	}
	if s.ObjectFormat != "obd" {
		s.ObjectFormat = "otobj"
	}
	switch s.UITheme {
	case ThemeClassic, ThemeModernDark, ThemeModernLight:
	default:
		s.UITheme = ThemeModernDark
	}
	if s.UIScale == 0 {
		s.UIScale = DefaultUIScale
	}
	s.UIScale = min(max(s.UIScale, MinUIScale), MaxUIScale)
	s.WorkerThreads = min(max(s.WorkerThreads, 0), 32)
	if s.UndoQueueSize == 0 {
		s.UndoQueueSize = DefaultUndoQueue
	}
	s.UndoQueueSize = min(max(s.UndoQueueSize, 64), 65536)
	if s.UndoMaxMemoryMB == 0 {
		s.UndoMaxMemoryMB = DefaultUndoMaxMB
	}
	s.UndoMaxMemoryMB = min(max(s.UndoMaxMemoryMB, 128), 16384)
	if s.ListColumns == 0 {
		s.ListColumns = DefaultListColumns
	}
	s.ListColumns = min(max(s.ListColumns, MinListColumns), MaxListColumns)
	if s.MCPPort <= 0 || s.MCPPort > 0xFFFF {
		s.MCPPort = DefaultMCPPort
	}
	if s.Recent == nil {
		s.Recent = []Recent{}
	}
	if s.Workspaces == nil {
		s.Workspaces = []WorkspaceProfile{}
	}
	for i := range s.Workspaces {
		switch s.Workspaces[i].PreferredItemIds {
		case ItemIDsAuto, ItemIDsClient, ItemIDsServer:
		default:
			s.Workspaces[i].PreferredItemIds = ItemIDsAuto
		}
	}
	if len(s.Workspaces) > MaxWorkspaces {
		s.Workspaces = s.Workspaces[:MaxWorkspaces]
	}
	if s.MarketShared == nil {
		s.MarketShared = map[string]string{}
	}
	if s.LastDirs == nil {
		s.LastDirs = map[string]string{}
	}
	if s.ItemsOTB == nil {
		s.ItemsOTB = map[string]string{}
	}
	if len(s.Recent) > MaxRecent {
		s.Recent = s.Recent[:MaxRecent]
	}
}

type Store struct {
	mu       sync.Mutex
	path     string
	settings Settings
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "OTS Creator", "settings.json"), nil
}

// Open loads settings from path. A missing or unreadable file gives the
// defaults; the error reports a corrupt file but the store still works.
func Open(path string) (*Store, error) {
	store := &Store{path: path, settings: Defaults()}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return store, err
	}
	loaded := Defaults()
	// Existing settings.json files created before uiTheme was added stay on
	// Classic so existing users keep their original look until they switch.
	loaded.UITheme = ""
	if err := json.Unmarshal(data, &loaded); err != nil {
		return store, fsutil.SetAside(path, err)
	}
	if loaded.UITheme == "" {
		loaded.UITheme = ThemeClassic
	}
	loaded.normalize()
	store.settings = loaded
	return store, nil
}

func (st *Store) Get() Settings {
	st.mu.Lock()
	defer st.mu.Unlock()
	snapshot := st.settings
	snapshot.Recent = append([]Recent{}, st.settings.Recent...)
	snapshot.Workspaces = append([]WorkspaceProfile{}, st.settings.Workspaces...)
	snapshot.MarketShared = maps.Clone(st.settings.MarketShared)
	snapshot.LastDirs = maps.Clone(st.settings.LastDirs)
	snapshot.ItemsOTB = maps.Clone(st.settings.ItemsOTB)
	return snapshot
}

// Update changes the preferences. The recent list, saved workspaces, shared
// market entries and dialog folders are kept as stored.
func (st *Store) Update(updated Settings) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	updated.Recent = st.settings.Recent
	updated.Workspaces = st.settings.Workspaces
	updated.ActiveWorkspace = st.settings.ActiveWorkspace
	updated.MarketShared = st.settings.MarketShared
	updated.LastDirs = st.settings.LastDirs
	updated.ItemsOTB = st.settings.ItemsOTB
	updated.normalize()
	st.settings = updated
	return st.saveLocked()
}

func (st *Store) AddRecent(recent Recent) error {
	if recent.DatPath == "" {
		return nil
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if recent.OpenedAt.IsZero() {
		recent.OpenedAt = time.Now()
	}
	st.settings.Recent = fsutil.AppendOtherPaths([]Recent{recent}, st.settings.Recent, recent.DatPath, recentDatPath)
	st.settings.normalize()
	return st.saveLocked()
}

// SetMarketShared remembers the delete token of a shared market entry;
// an empty token forgets the entry.
func (st *Store) SetMarketShared(id, token string) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	if token == "" {
		delete(st.settings.MarketShared, id)
	} else {
		st.settings.MarketShared[id] = token
	}
	return st.saveLocked()
}

func (st *Store) SetMarketAuthor(name string) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.settings.MarketAuthor == name {
		return nil
	}
	st.settings.MarketAuthor = name
	return st.saveLocked()
}

func (st *Store) LastDir(key string) string {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.settings.LastDirs[key]
}

func (st *Store) SetLastDir(key, dir string) error {
	if key == "" || dir == "" {
		return nil
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.settings.LastDirs[key] == dir {
		return nil
	}
	st.settings.LastDirs[key] = dir
	return st.saveLocked()
}

func (st *Store) RemoveRecent(datPath string) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	list := []Recent{}
	if datPath != "" {
		list = fsutil.AppendOtherPaths(list, st.settings.Recent, datPath, recentDatPath)
	}
	st.settings.Recent = list
	return st.saveLocked()
}

func recentDatPath(recent Recent) string { return recent.DatPath }

func (st *Store) ItemsOTB(clientPath string) string {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.settings.ItemsOTB[fsutil.PathKey(clientPath)]
}

func (st *Store) SetItemsOTB(clientPath, otbPath string) error {
	if clientPath == "" {
		return nil
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.settings.ItemsOTB == nil {
		st.settings.ItemsOTB = map[string]string{}
	}
	if otbPath == "" {
		delete(st.settings.ItemsOTB, fsutil.PathKey(clientPath))
	} else {
		st.settings.ItemsOTB[fsutil.PathKey(clientPath)] = otbPath
	}
	return st.saveLocked()
}

func (st *Store) SaveWorkspace(profile WorkspaceProfile) ([]WorkspaceProfile, error) {
	if profile.ID == "" {
		return st.Get().Workspaces, nil
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if profile.LastUsedAt == "" {
		profile.LastUsedAt = time.Now().UTC().Format(time.RFC3339)
	}
	next := []WorkspaceProfile{profile}
	for _, existing := range st.settings.Workspaces {
		if existing.ID != profile.ID {
			next = append(next, existing)
		}
	}
	st.settings.Workspaces = next
	st.settings.ActiveWorkspace = profile.ID
	st.settings.normalize()
	err := st.saveLocked()
	return append([]WorkspaceProfile{}, st.settings.Workspaces...), err
}

func (st *Store) RenameWorkspace(id, name string) ([]WorkspaceProfile, error) {
	if id == "" || name == "" {
		return st.Get().Workspaces, nil
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	changed := false
	for i := range st.settings.Workspaces {
		if st.settings.Workspaces[i].ID == id {
			st.settings.Workspaces[i].Name = name
			changed = true
			break
		}
	}
	if !changed {
		return append([]WorkspaceProfile{}, st.settings.Workspaces...), nil
	}
	err := st.saveLocked()
	return append([]WorkspaceProfile{}, st.settings.Workspaces...), err
}

func (st *Store) RemoveWorkspace(id string) ([]WorkspaceProfile, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	next := make([]WorkspaceProfile, 0, len(st.settings.Workspaces))
	for _, existing := range st.settings.Workspaces {
		if existing.ID != id {
			next = append(next, existing)
		}
	}
	st.settings.Workspaces = next
	if st.settings.ActiveWorkspace == id {
		if len(next) > 0 {
			st.settings.ActiveWorkspace = next[0].ID
		} else {
			st.settings.ActiveWorkspace = ""
		}
	}
	err := st.saveLocked()
	return append([]WorkspaceProfile{}, st.settings.Workspaces...), err
}

func (st *Store) SetActiveWorkspace(id string) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.settings.ActiveWorkspace == id {
		return nil
	}
	st.settings.ActiveWorkspace = id
	return st.saveLocked()
}

func (st *Store) saveLocked() error {
	return fsutil.WriteJSONPrivate(st.path, st.settings, "  ")
}
