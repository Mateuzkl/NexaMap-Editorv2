package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultsWhenMissing(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "none", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if s := st.Get(); !s.CheckUpdates || s.SheetBackground != BackgroundMagenta || s.ExportFormat != "png" || s.ListColumns != DefaultListColumns || s.Recent == nil {
		t.Fatalf("defaults %+v", s)
	}
}

func TestRoundTripAndRecent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfg", "settings.json")
	st, _ := Open(path)
	s := st.Get()
	s.CheckUpdates = false
	s.ExportFormat = "bogus"
	s.ListColumns = 99
	s.Recent = nil // ignored by Update
	if err := st.Update(s); err != nil {
		t.Fatal(err)
	}
	for i := range MaxRecent + 2 {
		st.AddRecent(Recent{DatPath: filepath.Join("c", string(rune('a'+i)), "Tibia.dat")})
	}
	st.AddRecent(Recent{DatPath: filepath.Join("c", "c", "Tibia.dat")})

	re, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got := re.Get()
	if got.CheckUpdates || got.ExportFormat != "png" || got.ListColumns != MaxListColumns {
		t.Fatalf("prefs %+v", got)
	}
	if len(got.Recent) != MaxRecent || got.Recent[0].DatPath != filepath.Join("c", "c", "Tibia.dat") {
		t.Fatalf("recent %+v", got.Recent)
	}
	for _, r := range got.Recent[1:] {
		if r.DatPath == got.Recent[0].DatPath {
			t.Fatal("duplicate recent entry")
		}
	}

	re.RemoveRecent(got.Recent[0].DatPath)
	if n := len(re.Get().Recent); n != MaxRecent-1 {
		t.Fatalf("after remove %d", n)
	}
	re.RemoveRecent("")
	if n := len(re.Get().Recent); n != 0 {
		t.Fatalf("after clear %d", n)
	}
}

func TestCorruptFileKeepsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(path, []byte("{nope"), 0o644)
	st, err := Open(path)
	if err == nil {
		t.Fatal("corrupt file must report an error")
	}
	if !st.Get().CheckUpdates {
		t.Fatal("store must fall back to defaults")
	}
}

func TestLastDirs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	st, _ := Open(path)
	if err := st.SetLastDir("openClient", filepath.Join("c", "client")); err != nil {
		t.Fatal(err)
	}
	s := st.Get()
	s.LastDirs = nil // ignored by Update
	if err := st.Update(s); err != nil {
		t.Fatal(err)
	}
	re, _ := Open(path)
	if got := re.LastDir("openClient"); got != filepath.Join("c", "client") {
		t.Fatalf("openClient = %q", got)
	}
	if got := re.LastDir("exportSprites"); got != "" {
		t.Fatalf("exportSprites = %q", got)
	}
}

func TestUIThemeDefaultsAndMigration(t *testing.T) {
	// New user gets Modern Dark by default.
	fresh, err := Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fresh.Get().UITheme; got != ThemeModernDark {
		t.Fatalf("new user uiTheme = %q, want %q", got, ThemeModernDark)
	}

	// Legacy settings.json without uiTheme migrates to Classic without losing data.
	legacyPath := filepath.Join(t.TempDir(), "legacy.json")
	legacyJSON := `{
		"checkUpdates": false,
		"listColumns": 9,
		"recent": [{"datPath": "/old/Tibia.dat", "sprPath": "/old/Tibia.spr"}],
		"marketShared": {"m1": "tok1"},
		"lastDirs": {"openClient": "/old"},
		"itemsOtb": {"/old/tibia.dat": "/srv/items.otb"}
	}`
	if err := os.WriteFile(legacyPath, []byte(legacyJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	legacy, err := Open(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	got := legacy.Get()
	if got.UITheme != ThemeModernDark {
		t.Fatalf("legacy uiTheme = %q, want %q", got.UITheme, ThemeModernDark)
	}
	if got.CheckUpdates || got.ListColumns != 9 || len(got.Recent) != 1 || got.MarketShared["m1"] != "tok1" || got.LastDirs["openClient"] != "/old" {
		t.Fatalf("legacy settings lost fields: %+v", got)
	}

	// Unknown theme value falls back safely.
	badPath := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(badPath, []byte(`{"uiTheme":"neon-invalid"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	bad, err := Open(badPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := bad.Get().UITheme; got != ThemeModernDark {
		t.Fatalf("invalid uiTheme fallback = %q, want %q", got, ThemeModernDark)
	}
}

func TestWorkspacesPersistenceAndProtection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	st, _ := Open(path)
	p1 := WorkspaceProfile{ID: "ws-1", Name: "My 8.60", ClientRoot: "/c1", ServerRoot: "/s1", PreferredItemIds: ItemIDsServer}
	p2 := WorkspaceProfile{ID: "ws-2", Name: "Canary", ClientRoot: "/c2", ServerRoot: "/s2", PreferredItemIds: "bogus"}
	if _, err := st.SaveWorkspace(p1); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveWorkspace(p2); err != nil {
		t.Fatal(err)
	}

	// Update must preserve workspaces and active workspace.
	s := st.Get()
	s.UITheme = ThemeModernLight
	s.Workspaces = nil
	s.ActiveWorkspace = ""
	if err := st.Update(s); err != nil {
		t.Fatal(err)
	}

	re, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got := re.Get()
	if got.UITheme != ThemeModernLight {
		t.Fatalf("uiTheme = %q", got.UITheme)
	}
	if len(got.Workspaces) != 2 || got.Workspaces[0].ID != "ws-2" || got.Workspaces[0].PreferredItemIds != ItemIDsAuto {
		t.Fatalf("workspaces = %+v", got.Workspaces)
	}
	if got.ActiveWorkspace != "ws-2" {
		t.Fatalf("activeWorkspace = %q", got.ActiveWorkspace)
	}

	if _, err := re.RenameWorkspace("ws-1", "TFS 8.60"); err != nil {
		t.Fatal(err)
	}
	if _, err := re.RemoveWorkspace("ws-2"); err != nil {
		t.Fatal(err)
	}
	after := re.Get()
	if len(after.Workspaces) != 1 || after.Workspaces[0].Name != "TFS 8.60" || after.ActiveWorkspace != "ws-1" {
		t.Fatalf("after rename/remove = %+v", after)
	}
}
