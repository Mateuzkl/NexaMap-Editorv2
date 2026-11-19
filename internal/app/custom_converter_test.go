package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nekiro/ots-creator/internal/core/otitems"
	"github.com/nekiro/ots-creator/internal/formats/itemsotb"
	"github.com/nekiro/ots-creator/internal/formats/otbnode"
	"github.com/nekiro/ots-creator/internal/mapping/otmap"
)

func sampleConverterOTB(t *testing.T, pairs [][2]uint16) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := otbnode.NewWriter(&buf, [4]byte{})
	w.Start(0)
	w.U32(0)
	w.U8(1)
	w.U16(140)
	w.U32(3)
	w.U32(20)
	w.U32(100)
	w.Bytes(make([]byte, 128))
	for _, p := range pairs {
		w.Start(byte(itemsotb.GroupGround))
		w.U32(0)
		w.U8(itemsotb.AttrServerID)
		w.U16(2)
		w.U16(p[0])
		w.U8(itemsotb.AttrClientID)
		w.U16(2)
		w.U16(p[1])
		w.End()
	}
	w.End()
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestCustomServerConverterEndToEnd(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	itemsDir := filepath.Join(src, "data", "items")
	worldDir := filepath.Join(src, "data", "world")
	for _, d := range []string{itemsDir, worldDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// ServerID 100 -> ClientID 200, ServerID 101 -> ClientID 201
	otbBytes := sampleConverterOTB(t, [][2]uint16{{100, 200}, {101, 201}})
	otbPath := filepath.Join(itemsDir, "items.otb")
	if err := os.WriteFile(otbPath, otbBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	xmlContent := `<?xml version="1.0"?><items><item id="100" name="grass"/><item fromid="100" toid="101" name="range"/></items>`
	xmlPath := filepath.Join(itemsDir, "items.xml")
	if err := os.WriteFile(xmlPath, []byte(xmlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Build a minimal OTBM map with ServerID 100 on ground and 101 on tile items.
	otbDB, err := otitems.ParseOTB(otbBytes)
	if err != nil {
		t.Fatal(err)
	}
	gm := otmap.New(64, 64)
	gm.SetTile(otmap.Position{X: 10, Y: 10, Z: 7}, &otmap.Tile{
		Ground: otmap.Item{ID: 100},
		Items:  []otmap.Item{{ID: 101}},
	})
	mapPath := filepath.Join(worldDir, "world.otbm")
	if _, err := otmap.Save(mapPath, gm, otbDB, otmap.Format{}); err != nil {
		t.Fatal(err)
	}

	cs := NewConverterService()
	scope := CustomServerScope{Maps: true, ItemsXML: true, ItemsOTB: true}

	analysis := cs.AnalyzeCustomServer(src, scope, false)
	if !analysis.Ready || analysis.Stats.MappedItems != 2 {
		t.Fatalf("analysis = %+v", analysis)
	}

	// Guard: same source and destination must fail.
	same := cs.ConvertCustomServer(CustomServerRequest{SourceRoot: src, DestinationRoot: src, Scope: scope})
	if same.Success || same.Error == "" {
		t.Fatalf("expected error when src == dst, got %+v", same)
	}

	report := cs.ConvertCustomServer(CustomServerRequest{SourceRoot: src, DestinationRoot: dst, Scope: scope})
	if !report.Success || report.Error != "" {
		t.Fatalf("convert failed: %+v", report)
	}
	if len(report.Maps) != 1 || report.Maps[0].ChangedItems != 2 {
		t.Fatalf("map report = %+v", report.Maps)
	}
	if report.ItemsXML == nil || report.ItemsXML.UpdatedTags != 2 {
		t.Fatalf("xml report = %+v", report.ItemsXML)
	}
	if report.ItemsOTB == nil || report.ItemsOTB.Remapped != 2 {
		t.Fatalf("otb report = %+v", report.ItemsOTB)
	}

	// Verify converted XML contains Client IDs 200 and 201.
	dstXML, err := os.ReadFile(filepath.Join(dst, "data", "items", "items.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dstXML), `id="200"`) || !strings.Contains(string(dstXML), `fromid="200" toid="201"`) {
		t.Fatalf("unexpected converted xml: %s", string(dstXML))
	}

	// Verify converted items.otb parses and maps 200/201 as grounds, and converted OTBM loads cleanly with it.
	dstOTBBytes, err := os.ReadFile(filepath.Join(dst, "data", "items", "items.otb"))
	if err != nil {
		t.Fatal(err)
	}
	dstDB, err := otitems.ParseOTB(dstOTBBytes)
	if err != nil {
		t.Fatal(err)
	}
	loaded, _, err := otmap.Load(filepath.Join(dst, "data", "world", "world.otbm"), dstDB, otmap.Format{})
	if err != nil {
		t.Fatal(err)
	}
	gotTile := loaded.Tile(otmap.Position{X: 10, Y: 10, Z: 7})
	if gotTile == nil || gotTile.Ground.ID != 200 || len(gotTile.Items) != 1 || gotTile.Items[0].ID != 201 {
		t.Fatalf("converted tile = %+v", gotTile)
	}

	// Verify source XML is untouched.
	srcXMLAfter, _ := os.ReadFile(xmlPath)
	if string(srcXMLAfter) != xmlContent {
		t.Fatalf("source items.xml was modified")
	}
}
