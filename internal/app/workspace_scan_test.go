package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nekiro/ots-creator/internal/formats/itemsotb"
	"github.com/nekiro/ots-creator/internal/formats/otbnode"
)

func minimalValidOTB(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := otbnode.NewWriter(&buf, [4]byte{})
	w.Start(0)
	w.U32(0)
	w.U8(1) // rootAttrVersion
	w.U16(140)
	w.U32(3)
	w.U32(20)
	w.U32(100)
	w.Bytes(make([]byte, 128))
	w.Start(byte(itemsotb.GroupGround))
	w.U32(0)
	w.U8(itemsotb.AttrServerID)
	w.U16(2)
	w.U16(100)
	w.U8(itemsotb.AttrClientID)
	w.U16(2)
	w.U16(100)
	w.End()
	w.End()
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestScanServerRootTFSLayout(t *testing.T) {
	root := t.TempDir()
	mustMkdir := func(rel string) {
		if err := os.MkdirAll(filepath.Join(root, rel), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite := func(rel string, data []byte) {
		if err := os.WriteFile(filepath.Join(root, rel), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	mustMkdir(filepath.Join("data", "items"))
	mustMkdir(filepath.Join("data", "world"))
	mustMkdir(filepath.Join("data", "monster", "undead"))
	mustMkdir(filepath.Join("data", "npc"))
	mustMkdir(filepath.Join("data", "actions"))
	mustMkdir(filepath.Join("logs"))
	mustMkdir(filepath.Join(".git"))

	otbBytes := minimalValidOTB(t)
	xmlBytes := []byte(`<?xml version="1.0"?><items><item id="100" name="void"/></items>`)

	mustWrite("config.lua", []byte(`mapName = "world"`))
	mustWrite(filepath.Join("data", "items", "items.otb"), otbBytes)
	mustWrite(filepath.Join("data", "items", "items.xml"), xmlBytes)
	mustWrite(filepath.Join("data", "world", "world.otbm"), []byte("OTBM"))
	mustWrite(filepath.Join("data", "world", "event área.otmap"), []byte("OTMAP"))
	mustWrite(filepath.Join("data", "monster", "undead", "rat.xml"), []byte("<monster/>"))
	mustWrite(filepath.Join("data", "npc", "guide.xml"), []byte("<npc/>"))
	// Ignored folders must not show up in maps.
	mustWrite(filepath.Join("logs", "backup.otbm"), []byte("ignored"))
	mustWrite(filepath.Join(".git", "bad.otbm"), []byte("ignored"))

	otbPath := filepath.Join(root, "data", "items", "items.otb")
	beforeInfo, err := os.Stat(otbPath)
	if err != nil {
		t.Fatal(err)
	}

	res := scanServerRoot(context.Background(), root, "", "")
	if res.State != ResourceFound {
		t.Fatalf("state = %q, err = %q", res.State, res.Error)
	}
	if res.Profile != "TFS" {
		t.Fatalf("profile = %q, want TFS", res.Profile)
	}
	if res.ItemsOTB.State != ResourceFound || res.ItemsOTB.Path != otbPath {
		t.Fatalf("itemsOtb = %+v", res.ItemsOTB)
	}
	if res.ItemsXML.State != ResourceFound {
		t.Fatalf("itemsXml = %+v", res.ItemsXML)
	}
	if len(res.Maps) != 2 {
		t.Fatalf("maps = %+v, want 2", res.Maps)
	}
	if res.Maps[0].Name != "event área.otmap" || res.Maps[0].Format != "otmap" {
		t.Fatalf("first map = %+v", res.Maps[0])
	}
	if res.Maps[1].Name != "world.otbm" || res.Maps[1].Format != "otbm" {
		t.Fatalf("second map = %+v", res.Maps[1])
	}
	if res.Monsters.State != ResourceFound || res.NPCs.State != ResourceFound {
		t.Fatalf("creatures = %+v / %+v", res.Monsters, res.NPCs)
	}

	// Read-only guarantee: file content and modtime untouched.
	afterInfo, err := os.Stat(otbPath)
	if err != nil {
		t.Fatal(err)
	}
	if !afterInfo.ModTime().Equal(beforeInfo.ModTime()) || afterInfo.Size() != beforeInfo.Size() {
		t.Fatalf("scan modified source file")
	}
}

func TestScanServerRootAlternativeAndPartial(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "data", "maps"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "data", "items.xml"), []byte(`<items></items>`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "data", "maps", "main.otbm"), []byte("OTBM"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := scanServerRoot(context.Background(), root, "", "")
	if res.State != ResourceFound {
		t.Fatalf("state = %q", res.State)
	}
	if res.ItemsOTB.State != ResourceMissing {
		t.Fatalf("otb state = %q, want missing", res.ItemsOTB.State)
	}
	if res.ItemsXML.State != ResourceFound {
		t.Fatalf("xml state = %q, want found", res.ItemsXML.State)
	}
	if res.Profile != "Unknown/Generic" {
		t.Fatalf("profile = %q, want Unknown/Generic", res.Profile)
	}
	if len(res.Maps) != 1 || res.Maps[0].Name != "main.otbm" {
		t.Fatalf("maps = %+v", res.Maps)
	}
}

func TestScanServerRootInvalidAndMissing(t *testing.T) {
	// Missing folder
	missing := scanServerRoot(context.Background(), filepath.Join(t.TempDir(), "does-not-exist"), "", "")
	if missing.State != ResourceMissing || missing.Error == "" {
		t.Fatalf("missing = %+v", missing)
	}

	// Empty folder
	empty := scanServerRoot(context.Background(), t.TempDir(), "", "")
	if empty.State != ResourceMissing {
		t.Fatalf("empty = %+v", empty)
	}

	// Invalid/corrupt OTB and XML
	bad := t.TempDir()
	if err := os.WriteFile(filepath.Join(bad, "items.otb"), []byte("not-an-otb"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "items.xml"), []byte("<html>not items</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := scanServerRoot(context.Background(), bad, "", "")
	if res.ItemsOTB.State != ResourceInvalid || res.ItemsOTB.Error == "" {
		t.Fatalf("bad otb = %+v", res.ItemsOTB)
	}
	if res.ItemsXML.State != ResourceInvalid || res.ItemsXML.Error == "" {
		t.Fatalf("bad xml = %+v", res.ItemsXML)
	}
}

func TestScanServerRootCancellation(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res := scanServerRoot(ctx, root, "", "")
	if res.State != ResourceMissing {
		t.Fatalf("canceled empty root state = %q", res.State)
	}
}

func TestInspectClientWorkspaceAmbiguity(t *testing.T) {
	dir := t.TempDir()
	// Two distinct clients in the same directory -> must report ambiguity, not silently pick one.
	if err := os.WriteFile(filepath.Join(dir, "ClientA.dat"), []byte{1, 2, 3, 4}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ClientB.dat"), []byte{5, 6, 7, 8}, 0o644); err != nil {
		t.Fatal(err)
	}
	got := inspectClientWorkspace(dir, "", nil)
	if !got.Ambiguous || len(got.Candidates) != 2 {
		t.Fatalf("expected ambiguity with 2 candidates, got %+v", got)
	}

	// Missing folder -> ResourceMissing
	miss := inspectClientWorkspace(filepath.Join(dir, "nope"), "", nil)
	if miss.State != ResourceMissing {
		t.Fatalf("missing client = %+v", miss)
	}
}
