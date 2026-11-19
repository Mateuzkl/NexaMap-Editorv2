package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/nekiro/ots-creator/internal/core/otitems"
	"github.com/nekiro/ots-creator/internal/formats/itemsotb"
	"github.com/nekiro/ots-creator/internal/fsutil"
	"github.com/nekiro/ots-creator/internal/mapping/otmap"
)

type ConverterService struct{}

func NewConverterService() *ConverterService { return &ConverterService{} }

type CustomServerScope struct {
	Maps     bool `json:"maps"`
	ItemsXML bool `json:"itemsXml"`
	ItemsOTB bool `json:"itemsOtb"`
}

type CustomServerMappingStats struct {
	TotalItems         int `json:"totalItems"`
	MappedItems        int `json:"mappedItems"`
	UnmappedItems      int `json:"unmappedItems"`
	DuplicateClientIDs int `json:"duplicateClientIds"`
}

type CustomServerAnalysis struct {
	SourceRoot               string                   `json:"sourceRoot"`
	Scope                    CustomServerScope        `json:"scope"`
	Server                   ServerResources          `json:"server"`
	Stats                    CustomServerMappingStats `json:"stats"`
	DuplicateClientIDsAllowed bool                    `json:"duplicateClientIdsAllowed"`
	Warnings                 []string                 `json:"warnings"`
	Ready                    bool                     `json:"ready"`
	Error                    string                   `json:"error,omitempty"`
}

type CustomServerRequest struct {
	SourceRoot              string            `json:"sourceRoot"`
	DestinationRoot         string            `json:"destinationRoot"`
	Scope                   CustomServerScope `json:"scope"`
	AllowDuplicateClientIDs bool              `json:"allowDuplicateClientIds"`
}

type CustomMapReport struct {
	Name         string `json:"name"`
	Source       string `json:"source"`
	Destination  string `json:"destination"`
	TileCount    int    `json:"tileCount"`
	TotalItems   int    `json:"totalItems"`
	MappedItems  int    `json:"mappedItems"`
	ChangedItems int    `json:"changedItems"`
	MissingItems int    `json:"missingItems"`
	Error        string `json:"error,omitempty"`
}

type CustomXMLReport struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	TotalTags   int    `json:"totalTags"`
	UpdatedTags int    `json:"updatedTags"`
	SplitRanges int    `json:"splitRanges"`
	Unmapped    int    `json:"unmapped"`
	Error       string `json:"error,omitempty"`
}

type CustomOTBReport struct {
	Source       string `json:"source"`
	Destination  string `json:"destination"`
	TotalItems   int    `json:"totalItems"`
	Remapped     int    `json:"remapped"`
	Skipped      int    `json:"skipped"`
	Error        string `json:"error,omitempty"`
}

type CustomServerReport struct {
	Success    bool              `json:"success"`
	DurationMs int64             `json:"durationMs"`
	Maps       []CustomMapReport `json:"maps"`
	ItemsXML   *CustomXMLReport  `json:"itemsXml,omitempty"`
	ItemsOTB   *CustomOTBReport  `json:"itemsOtb,omitempty"`
	Warnings   []string          `json:"warnings"`
	Error      string            `json:"error,omitempty"`
}

func (cs *ConverterService) AnalyzeCustomServer(sourceRoot string, scope CustomServerScope, allowDuplicateClientIDs bool) CustomServerAnalysis {
	out := CustomServerAnalysis{
		SourceRoot:                filepath.Clean(strings.TrimSpace(sourceRoot)),
		Scope:                     scope,
		DuplicateClientIDsAllowed: allowDuplicateClientIDs,
		Warnings:                  []string{},
	}
	if strings.TrimSpace(sourceRoot) == "" {
		out.Error = "Select a source server folder first."
		return out
	}
	if !scope.Maps && !scope.ItemsXML && !scope.ItemsOTB {
		out.Error = "Enable at least one conversion target (Maps, items.xml, or items.otb)."
		return out
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	server := scanServerRoot(ctx, sourceRoot, "", "")
	out.Server = server
	if server.State != ResourceFound {
		out.Error = server.Error
		if out.Error == "" {
			out.Error = "No server resources found in the selected folder."
		}
		return out
	}
	if server.ItemsOTB.State != ResourceFound {
		out.Error = "A valid items.otb is required to map Server IDs to Client IDs."
		if server.ItemsOTB.Error != "" {
			out.Error += " (" + server.ItemsOTB.Error + ")"
		}
		return out
	}

	otbData, err := os.ReadFile(server.ItemsOTB.Path)
	if err != nil {
		out.Error = fmt.Sprintf("Failed to read items.otb: %v", err)
		return out
	}
	otbFile, err := itemsotb.Parse(otbData)
	if err != nil {
		out.Error = fmt.Sprintf("Failed to parse items.otb: %v", err)
		return out
	}

	_, stats := buildServerToClientMap(otbFile)
	out.Stats = stats
	if stats.DuplicateClientIDs > 0 {
		msg := fmt.Sprintf("%d Server IDs share the same Client ID in items.otb.", stats.DuplicateClientIDs)
		if !allowDuplicateClientIDs {
			out.Error = msg + " Enable 'Allow duplicate ClientIDs' to proceed anyway."
			return out
		}
		out.Warnings = append(out.Warnings, msg)
	}
	if scope.Maps && len(server.Maps) == 0 {
		out.Warnings = append(out.Warnings, "No .otbm or .otmap maps found in source server.")
	}
	if scope.ItemsXML && server.ItemsXML.State != ResourceFound {
		out.Warnings = append(out.Warnings, "items.xml not found or invalid; XML conversion will be skipped.")
	}

	out.Ready = true
	return out
}

func (cs *ConverterService) ConvertCustomServer(req CustomServerRequest) CustomServerReport {
	started := time.Now()
	report := CustomServerReport{
		Maps:     []CustomMapReport{},
		Warnings: []string{},
	}

	src := filepath.Clean(strings.TrimSpace(req.SourceRoot))
	dst := filepath.Clean(strings.TrimSpace(req.DestinationRoot))
	if src == "" || dst == "" {
		report.Error = "Both source server folder and destination folder are required."
		return report
	}
	if fsutil.SamePath(src, dst) {
		report.Error = "Destination folder must be different from the source server folder (source files are never overwritten)."
		return report
	}

	analysis := cs.AnalyzeCustomServer(src, req.Scope, req.AllowDuplicateClientIDs)
	if !analysis.Ready {
		report.Error = analysis.Error
		return report
	}
	report.Warnings = append(report.Warnings, analysis.Warnings...)

	otbData, err := os.ReadFile(analysis.Server.ItemsOTB.Path)
	if err != nil {
		report.Error = fmt.Sprintf("Read items.otb: %v", err)
		return report
	}
	otbFile, err := itemsotb.Parse(otbData)
	if err != nil {
		report.Error = fmt.Sprintf("Parse items.otb: %v", err)
		return report
	}
	mapping, _ := buildServerToClientMap(otbFile)
	otbDB := otitems.FromOTB(otbFile)

	if err := os.MkdirAll(dst, 0o755); err != nil {
		report.Error = fmt.Sprintf("Create destination folder: %v", err)
		return report
	}

	// 1. Convert Maps (.otbm / .otmap)
	if req.Scope.Maps {
		for _, m := range analysis.Server.Maps {
			rel, err := filepath.Rel(src, m.Path)
			if err != nil || strings.HasPrefix(rel, "..") {
				rel = filepath.Join("data", "world", m.Name)
			}
			destPath := filepath.Join(dst, rel)
			mr := convertMapFile(m.Path, destPath, m.Name, otbDB, mapping)
			report.Maps = append(report.Maps, mr)
			if mr.Error != "" {
				report.Warnings = append(report.Warnings, fmt.Sprintf("%s: %s", m.Name, mr.Error))
			}
		}
	}

	// 2. Convert items.xml
	if req.Scope.ItemsXML && analysis.Server.ItemsXML.State == ResourceFound {
		xmlSrc := analysis.Server.ItemsXML.Path
		rel, err := filepath.Rel(src, xmlSrc)
		if err != nil || strings.HasPrefix(rel, "..") {
			rel = filepath.Join("data", "items", "items.xml")
		}
		xmlDst := filepath.Join(dst, rel)
		xr := convertItemsXMLFile(xmlSrc, xmlDst, mapping)
		report.ItemsXML = &xr
		if xr.Error != "" {
			report.Warnings = append(report.Warnings, "items.xml: "+xr.Error)
		}
	}

	// 3. Convert items.otb (ServerID -> ClientID)
	if req.Scope.ItemsOTB {
		otbSrc := analysis.Server.ItemsOTB.Path
		rel, err := filepath.Rel(src, otbSrc)
		if err != nil || strings.HasPrefix(rel, "..") {
			rel = filepath.Join("data", "items", "items.otb")
		}
		otbDst := filepath.Join(dst, rel)
		or := convertItemsOTBFile(otbSrc, otbDst, otbData)
		report.ItemsOTB = &or
		if or.Error != "" {
			report.Warnings = append(report.Warnings, "items.otb: "+or.Error)
		}
	}

	report.DurationMs = time.Since(started).Milliseconds()
	report.Success = true
	return report
}

func buildServerToClientMap(file *itemsotb.File) (map[uint16]uint16, CustomServerMappingStats) {
	mapping := make(map[uint16]uint16, len(file.Items))
	seenClient := make(map[uint16]uint16, len(file.Items))
	var stats CustomServerMappingStats
	for _, item := range file.Items {
		if item.Deprecated() {
			continue
		}
		sid := item.ServerID()
		if sid == 0 {
			continue
		}
		stats.TotalItems++
		cid := item.ClientID()
		if cid == 0 {
			stats.UnmappedItems++
			continue
		}
		mapping[sid] = cid
		stats.MappedItems++
		if prev, dup := seenClient[cid]; dup && prev != sid {
			stats.DuplicateClientIDs++
		} else {
			seenClient[cid] = sid
		}
	}
	return mapping, stats
}

func convertMapFile(srcPath, dstPath, name string, otbDB *otitems.DB, mapping map[uint16]uint16) CustomMapReport {
	mr := CustomMapReport{Name: name, Source: srcPath, Destination: dstPath}
	format := otmap.Format{}
	gameMap, _, err := otmap.Load(srcPath, otbDB, format)
	if err != nil {
		mr.Error = err.Error()
		return mr
	}

	var rewrite func(*otmap.Item)
	rewrite = func(item *otmap.Item) {
		if item.ID != 0 {
			mr.TotalItems++
			if cid, ok := mapping[item.ID]; ok && cid != 0 {
				mr.MappedItems++
				if cid != item.ID {
					mr.ChangedItems++
					item.ID = cid
				}
			} else {
				mr.MissingItems++
			}
		}
		if item.Ext != nil {
			for i := range item.Ext.Items {
				rewrite(&item.Ext.Items[i])
			}
		}
	}

	var changedTiles []struct {
		pos  otmap.Position
		tile *otmap.Tile
	}
	gameMap.Each(func(pos otmap.Position, tile *otmap.Tile) {
		mr.TileCount++
		before := mr.ChangedItems
		rewrite(&tile.Ground)
		for i := range tile.Items {
			rewrite(&tile.Items[i])
		}
		if mr.ChangedItems != before {
			changedTiles = append(changedTiles, struct {
				pos  otmap.Position
				tile *otmap.Tile
			}{pos: pos, tile: tile})
		}
	})
	for _, ct := range changedTiles {
		gameMap.SetTile(ct.pos, ct.tile)
	}

	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		mr.Error = err.Error()
		return mr
	}
	if _, err := otmap.Save(dstPath, gameMap, otbDB, format); err != nil {
		mr.Error = err.Error()
		return mr
	}
	return mr
}

var (
	reItemSingle = regexp.MustCompile(`(<item\b[^>]*?\bid\s*=\s*")(\d+)("[^>]*>)`)
	reItemRange  = regexp.MustCompile(`(<item\b[^>]*?\bfromid\s*=\s*")(\d+)("[^>]*?\btoid\s*=\s*")(\d+)("[^>]*>)`)
)

func convertItemsXMLFile(srcPath, dstPath string, mapping map[uint16]uint16) CustomXMLReport {
	xr := CustomXMLReport{Source: srcPath, Destination: dstPath}
	raw, err := os.ReadFile(srcPath)
	if err != nil {
		xr.Error = err.Error()
		return xr
	}
	text := string(raw)

	// Remap single <item id="..."> tags.
	text = reItemSingle.ReplaceAllStringFunc(text, func(match string) string {
		parts := reItemSingle.FindStringSubmatch(match)
		if len(parts) != 4 {
			return match
		}
		xr.TotalTags++
		sid, err := strconv.Atoi(parts[2])
		if err != nil || sid <= 0 || sid > 0xFFFF {
			xr.Unmapped++
			return match
		}
		cid, ok := mapping[uint16(sid)]
		if !ok || cid == 0 {
			xr.Unmapped++
			return match
		}
		if uint16(sid) != cid {
			xr.UpdatedTags++
		}
		return parts[1] + strconv.Itoa(int(cid)) + parts[3]
	})

	// Remap <item fromid="..." toid="..."> tags when endpoints map cleanly.
	text = reItemRange.ReplaceAllStringFunc(text, func(match string) string {
		parts := reItemRange.FindStringSubmatch(match)
		if len(parts) != 6 {
			return match
		}
		xr.TotalTags++
		fromID, err1 := strconv.Atoi(parts[2])
		toID, err2 := strconv.Atoi(parts[4])
		if err1 != nil || err2 != nil || fromID <= 0 || toID < fromID || toID > 0xFFFF {
			xr.Unmapped++
			return match
		}
		cFrom, ok1 := mapping[uint16(fromID)]
		cTo, ok2 := mapping[uint16(toID)]
		if !ok1 || !ok2 || cFrom == 0 || cTo == 0 || cTo < cFrom {
			xr.Unmapped++
			return match
		}
		if int(cTo-cFrom) != (toID - fromID) {
			xr.SplitRanges++
		}
		if uint16(fromID) != cFrom || uint16(toID) != cTo {
			xr.UpdatedTags++
		}
		return parts[1] + strconv.Itoa(int(cFrom)) + parts[3] + strconv.Itoa(int(cTo)) + parts[5]
	})

	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		xr.Error = err.Error()
		return xr
	}
	if err := os.WriteFile(dstPath, []byte(text), 0o644); err != nil {
		xr.Error = err.Error()
		return xr
	}
	return xr
}

func convertItemsOTBFile(srcPath, dstPath string, rawOTB []byte) CustomOTBReport {
	or := CustomOTBReport{Source: srcPath, Destination: dstPath}
	cloned, err := itemsotb.Parse(rawOTB)
	if err != nil {
		or.Error = err.Error()
		return or
	}
	for _, item := range cloned.Items {
		or.TotalItems++
		if !item.Deprecated() && item.ServerID() != 0 && item.ClientID() != 0 {
			if item.ServerID() != item.ClientID() {
				item.SetServerID(item.ClientID())
				or.Remapped++
			}
		} else {
			or.Skipped++
		}
	}
	encoded, err := cloned.Encode()
	if err != nil {
		or.Error = err.Error()
		return or
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		or.Error = err.Error()
		return or
	}
	if err := os.WriteFile(dstPath, encoded, 0o644); err != nil {
		or.Error = err.Error()
		return or
	}
	return or
}
