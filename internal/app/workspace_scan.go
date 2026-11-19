package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nekiro/ots-creator/internal/formats/assets"
	"github.com/nekiro/ots-creator/internal/formats/itemsotb"
	"github.com/nekiro/ots-creator/internal/fsutil"
)

type ResourceState string

const (
	ResourceNotConfigured ResourceState = "not-configured"
	ResourceFound         ResourceState = "found"
	ResourceMissing       ResourceState = "missing"
	ResourceInvalid       ResourceState = "invalid"
	ResourceUnsupported   ResourceState = "unsupported"
)

type ResourceResult struct {
	Path       string        `json:"path"`
	State      ResourceState `json:"state"`
	Details    string        `json:"details,omitempty"`
	Error      string        `json:"error,omitempty"`
	Candidates []string      `json:"candidates,omitempty"`
}

type ClientCandidate struct {
	Path  string `json:"path"`
	Kind  string `json:"kind"` // "otfi" | "dat" | "assets"
	Label string `json:"label"`
}

type ClientInspection struct {
	Root       string            `json:"root"`
	State      ResourceState     `json:"state"`
	Files      *ClientFiles      `json:"files,omitempty"`
	Candidates []ClientCandidate `json:"candidates,omitempty"`
	Ambiguous  bool              `json:"ambiguous,omitempty"`
	Error      string            `json:"error,omitempty"`
}

type DetectedMap struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Format string `json:"format"` // "otbm" | "otmap"
	Size   int64  `json:"size"`
	RelDir string `json:"relDir"`
}

type ServerResources struct {
	Root        string         `json:"root"`
	State       ResourceState  `json:"state"`
	Profile     string         `json:"profile"`
	ProfileNote string         `json:"profileNote,omitempty"`
	ServerDir   string         `json:"serverDir,omitempty"`
	ItemsOTB    ResourceResult `json:"itemsOtb"`
	ItemsXML    ResourceResult `json:"itemsXml"`
	Maps        []DetectedMap  `json:"maps"`
	Monsters    ResourceResult `json:"monsters"`
	NPCs        ResourceResult `json:"npcs"`
	Materials   ResourceResult `json:"materials"`
	Error       string         `json:"error,omitempty"`
}

type WorkspaceInspection struct {
	Generation int64            `json:"generation"`
	Client     ClientInspection `json:"client"`
	Server     ServerResources  `json:"server"`
}

const (
	workspaceMaxDepth   = 5
	workspaceMaxEntries = 4000
	workspaceMaxMaps    = 100
)

var ignoredScanDirs = map[string]bool{
	".git":         true,
	".hg":          true,
	".svn":         true,
	"node_modules": true,
	"build":        true,
	"dist":         true,
	"logs":         true,
	"log":          true,
	"cache":        true,
	".cache":       true,
	"backup":       true,
	"backups":      true,
	"tmp":          true,
	"temp":         true,
	"obj":          true,
	"bin":          true,
	".vscode":      true,
}

func inspectClientWorkspace(root, selectedClientPath string, rememberedOTB func(string) string) ClientInspection {
	cleaned := strings.TrimSpace(root)
	if cleaned == "" {
		return ClientInspection{State: ResourceNotConfigured}
	}
	cleaned = filepath.Clean(cleaned)
	info, err := os.Stat(cleaned)
	if err != nil {
		if os.IsNotExist(err) {
			return ClientInspection{Root: cleaned, State: ResourceMissing, Error: fmt.Sprintf("Client path not found: %s", cleaned)}
		}
		return ClientInspection{Root: cleaned, State: ResourceInvalid, Error: err.Error()}
	}

	candidates := findClientCandidates(cleaned, info.IsDir())
	inspectTarget := cleaned
	if selected := strings.TrimSpace(selectedClientPath); selected != "" {
		inspectTarget = filepath.Clean(selected)
	} else if len(candidates) > 1 {
		// Multiple distinct client files in the same folder: report ambiguity
		// instead of silently picking one.
		return ClientInspection{
			Root:       cleaned,
			State:      ResourceFound,
			Candidates: candidates,
			Ambiguous:  true,
			Error:      "Multiple client files found in this folder. Choose which client to use.",
		}
	} else if len(candidates) == 1 {
		inspectTarget = candidates[0].Path
	}

	files, err := inspectClient(inspectTarget)
	if err != nil {
		state := ResourceInvalid
		if os.IsNotExist(err) {
			state = ResourceMissing
		}
		return ClientInspection{
			Root:       cleaned,
			State:      state,
			Candidates: candidates,
			Error:      err.Error(),
		}
	}
	if rememberedOTB != nil {
		files.OTBPath = rememberedOTB(files.DatPath)
	}
	return ClientInspection{
		Root:       cleaned,
		State:      ResourceFound,
		Files:      &files,
		Candidates: candidates,
	}
}

func findClientCandidates(root string, isDir bool) []ClientCandidate {
	if !isDir {
		ext := strings.ToLower(filepath.Ext(root))
		kind := "dat"
		if ext == ".otfi" {
			kind = "otfi"
		}
		return []ClientCandidate{{Path: root, Kind: kind, Label: filepath.Base(root)}}
	}
	if _, err := os.ReadDir(root); err != nil {
		return nil
	}
	byBase := map[string]ClientCandidate{}
	collectDir := func(dir, labelPrefix string) {
		dirEntries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, entry := range dirEntries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			ext := strings.ToLower(filepath.Ext(name))
			if ext != ".otfi" && ext != ".dat" {
				continue
			}
			baseKey := strings.ToLower(filepath.Join(dir, fsutil.TrimExt(name)))
			full := filepath.Join(dir, name)
			label := name
			if labelPrefix != "" {
				label = filepath.ToSlash(filepath.Join(labelPrefix, name))
			}
			// An .otfi and .dat sharing a base name represent one client; .otfi wins.
			if prev, exists := byBase[baseKey]; !exists || (prev.Kind == "dat" && ext == ".otfi") {
				byBase[baseKey] = ClientCandidate{
					Path:  full,
					Kind:  strings.TrimPrefix(ext, "."),
					Label: label,
				}
			}
		}
	}
	collectDir(root, "")
	if len(byBase) == 0 {
		for _, sub := range []string{filepath.Join("data", "things"), "things"} {
			thingsDir := filepath.Join(root, sub)
			subEntries, err := os.ReadDir(thingsDir)
			if err != nil {
				continue
			}
			for _, subEntry := range subEntries {
				if subEntry.IsDir() {
					collectDir(filepath.Join(thingsDir, subEntry.Name()), filepath.Join(sub, subEntry.Name()))
				}
			}
		}
	}
	candidates := make([]ClientCandidate, 0, len(byBase)+1)
	for _, c := range byBase {
		candidates = append(candidates, c)
	}
	if assetsDir, err := assets.FindDir(root); err == nil {
		// Include assets folder as a candidate if there are no dat/otfi files,
		// or if catalog-content.json is directly inside root.
		if len(candidates) == 0 || fsutil.SamePath(assetsDir, root) {
			candidates = append(candidates, ClientCandidate{
				Path:  assetsDir,
				Kind:  "assets",
				Label: filepath.Base(assetsDir) + " (Tibia 12+ assets)",
			})
		}
	}
	slices.SortFunc(candidates, func(a, b ClientCandidate) int {
		if a.Kind != b.Kind {
			return strings.Compare(a.Kind, b.Kind)
		}
		return strings.Compare(strings.ToLower(a.Label), strings.ToLower(b.Label))
	})
	return candidates
}

func scanServerRoot(ctx context.Context, root, overrideOtb, overrideXml string) ServerResources {
	res := ServerResources{
		Profile:   "Unknown/Generic",
		ItemsOTB:  ResourceResult{State: ResourceNotConfigured},
		ItemsXML:  ResourceResult{State: ResourceNotConfigured},
		Maps:      []DetectedMap{},
		Monsters:  ResourceResult{State: ResourceNotConfigured},
		NPCs:      ResourceResult{State: ResourceNotConfigured},
		Materials: ResourceResult{State: ResourceNotConfigured},
	}
	cleaned := strings.TrimSpace(root)
	if cleaned == "" {
		res.State = ResourceNotConfigured
		return res
	}
	cleaned = filepath.Clean(cleaned)
	res.Root = cleaned
	info, err := os.Stat(cleaned)
	if err != nil {
		if os.IsNotExist(err) {
			res.State = ResourceMissing
			res.Error = fmt.Sprintf("Server folder not found: %s", cleaned)
		} else {
			res.State = ResourceInvalid
			res.Error = err.Error()
		}
		return res
	}
	if !info.IsDir() {
		cleaned = filepath.Dir(cleaned)
		res.Root = cleaned
	}

	resolvedRoot, _ := filepath.EvalSymlinks(cleaned)
	if resolvedRoot == "" {
		resolvedRoot = cleaned
	}

	// Baseline detection via existing server_data.go logic.
	baseData := detectServerData(cleaned)
	res.ServerDir = baseData.ServerDir
	hasCanaryDatapack := fsutil.DirExists(filepath.Join(cleaned, "data-otservbr-global")) ||
		fsutil.DirExists(filepath.Join(cleaned, "data-canary"))
	if hasCanaryDatapack {
		// Keep root so creature scanner walks both data/ and data-otservbr-global/data-canary.
		res.ServerDir = cleaned
	} else if res.ServerDir == "" {
		if fsutil.DirExists(filepath.Join(cleaned, "data")) {
			res.ServerDir = filepath.Join(cleaned, "data")
		} else {
			res.ServerDir = cleaned
		}
	}

	// Collect prioritized candidates for items.otb, items.xml, maps, materials.
	var otbCandidates []string
	var xmlCandidates []string
	var materialCandidates []string
	seenMaps := map[string]bool{}

	addUnique := func(list *[]string, p string) {
		p = filepath.Clean(p)
		for _, existing := range *list {
			if fsutil.SamePath(existing, p) {
				return
			}
		}
		*list = append(*list, p)
	}

	if baseData.OTBPath != "" {
		addUnique(&otbCandidates, baseData.OTBPath)
	}
	if baseData.XMLPath != "" {
		addUnique(&xmlCandidates, baseData.XMLPath)
	}

	// Check documented high-priority locations first.
	priorityItemDirs := []string{
		filepath.Join(cleaned, "data", "items"),
		filepath.Join(cleaned, "items"),
		filepath.Join(cleaned, "data"),
		cleaned,
	}
	for _, dir := range priorityItemDirs {
		if p := filepath.Join(dir, "items.otb"); fsutil.FileExists(p) {
			addUnique(&otbCandidates, p)
		}
		if p := filepath.Join(dir, "items.xml"); fsutil.FileExists(p) {
			addUnique(&xmlCandidates, p)
		}
	}

	// Bounded walk inside server root to discover additional OTB/XML/maps/materials.
	entriesVisited := 0
	_ = walkBounded(ctx, cleaned, resolvedRoot, workspaceMaxDepth, workspaceMaxEntries, &entriesVisited, func(path string, d fs.DirEntry, depth int) error {
		if d.IsDir() {
			return nil
		}
		lower := strings.ToLower(d.Name())
		switch {
		case lower == "items.otb":
			addUnique(&otbCandidates, path)
		case lower == "items.xml":
			addUnique(&xmlCandidates, path)
		case lower == "materials.xml" || lower == "brushes.json":
			addUnique(&materialCandidates, path)
		case strings.HasSuffix(lower, ".otbm") || strings.HasSuffix(lower, ".otmap"):
			if len(res.Maps) < workspaceMaxMaps {
				key := fsutil.PathKey(path)
				if !seenMaps[key] {
					seenMaps[key] = true
					var size int64
					if fi, err := d.Info(); err == nil {
						size = fi.Size()
					}
					rel, err := filepath.Rel(cleaned, filepath.Dir(path))
					if err != nil || rel == "." {
						rel = ""
					}
					format := "otbm"
					if strings.HasSuffix(lower, ".otmap") {
						format = "otmap"
					}
					res.Maps = append(res.Maps, DetectedMap{
						Name:   d.Name(),
						Path:   path,
						Format: format,
						Size:   size,
						RelDir: filepath.ToSlash(rel),
					})
				}
			}
		}
		return nil
	})

	sortDetectedMaps(res.Maps)

	// Validate OTB (override takes precedence if provided).
	otbTarget := strings.TrimSpace(overrideOtb)
	if otbTarget == "" && len(otbCandidates) > 0 {
		otbTarget = otbCandidates[0]
	}
	res.ItemsOTB = validateOTBResource(otbTarget, otbCandidates)

	// Validate XML (override takes precedence if provided).
	xmlTarget := strings.TrimSpace(overrideXml)
	if xmlTarget == "" && len(xmlCandidates) > 0 {
		xmlTarget = xmlCandidates[0]
	}
	res.ItemsXML = validateXMLResource(xmlTarget, xmlCandidates)

	// Monsters & NPCs directories.
	res.Monsters = scanCreatureDir(ctx, cleaned, resolvedRoot, []string{
		filepath.Join("data", "monster"),
		filepath.Join("data", "monsters"),
		"monster",
		"monsters",
	})
	res.NPCs = scanCreatureDir(ctx, cleaned, resolvedRoot, []string{
		filepath.Join("data", "npc"),
		filepath.Join("data", "npcs"),
		"npc",
		"npcs",
	})

	// Materials.
	if len(materialCandidates) > 0 {
		res.Materials = ResourceResult{
			Path:       materialCandidates[0],
			State:      ResourceFound,
			Details:    filepath.Base(materialCandidates[0]),
			Candidates: materialCandidates,
		}
	} else {
		res.Materials = ResourceResult{State: ResourceMissing, Details: "Optional — default brushes available"}
	}

	// Evidence-based server profile detection (never inferred from folder name).
	res.Profile, res.ProfileNote = detectServerProfile(cleaned, res.ItemsOTB.State == ResourceFound)

	if res.ItemsOTB.State == ResourceFound || res.ItemsXML.State == ResourceFound || len(res.Maps) > 0 || res.Monsters.State == ResourceFound || res.NPCs.State == ResourceFound {
		res.State = ResourceFound
	} else {
		res.State = ResourceMissing
		res.Error = "No recognized server resources (items.otb, items.xml, maps, monsters, or NPCs) found in this folder."
	}

	return res
}

func validateOTBResource(path string, candidates []string) ResourceResult {
	if path == "" {
		return ResourceResult{State: ResourceMissing, Details: "Not found", Candidates: candidates}
	}
	path = filepath.Clean(path)
	data, err := os.ReadFile(path)
	if err != nil {
		state := ResourceInvalid
		if os.IsNotExist(err) {
			state = ResourceMissing
		}
		return ResourceResult{Path: path, State: state, Error: err.Error(), Candidates: candidates}
	}
	if len(data) == 0 {
		return ResourceResult{Path: path, State: ResourceInvalid, Error: "items.otb is empty", Candidates: candidates}
	}
	parsed, err := itemsotb.Parse(data)
	if err != nil {
		return ResourceResult{Path: path, State: ResourceInvalid, Error: fmt.Sprintf("Invalid items.otb: %v", err), Candidates: candidates}
	}
	maj, min, build := parsed.Version()
	return ResourceResult{
		Path:       path,
		State:      ResourceFound,
		Details:    fmt.Sprintf("v%d.%d (build %d) · %d items", maj, min, build, len(parsed.Items)),
		Candidates: candidates,
	}
}

func validateXMLResource(path string, candidates []string) ResourceResult {
	if path == "" {
		return ResourceResult{State: ResourceMissing, Details: "Not found", Candidates: candidates}
	}
	path = filepath.Clean(path)
	file, err := os.Open(path)
	if err != nil {
		state := ResourceInvalid
		if os.IsNotExist(err) {
			state = ResourceMissing
		}
		return ResourceResult{Path: path, State: state, Error: err.Error(), Candidates: candidates}
	}
	defer file.Close()
	buf := make([]byte, 4096)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		return ResourceResult{Path: path, State: ResourceInvalid, Error: err.Error(), Candidates: candidates}
	}
	if n == 0 {
		return ResourceResult{Path: path, State: ResourceInvalid, Error: "items.xml is empty", Candidates: candidates}
	}
	lower := bytes.ToLower(buf[:n])
	if !bytes.Contains(lower, []byte("<items")) {
		return ResourceResult{Path: path, State: ResourceInvalid, Error: "File does not appear to be a valid items.xml (<items> tag not found)", Candidates: candidates}
	}
	return ResourceResult{
		Path:       path,
		State:      ResourceFound,
		Details:    "Valid XML header",
		Candidates: candidates,
	}
}

func scanCreatureDir(ctx context.Context, root, resolvedRoot string, relCandidates []string) ResourceResult {
	for _, rel := range relCandidates {
		dir := filepath.Join(root, rel)
		if !fsutil.DirExists(dir) {
			continue
		}
		count := 0
		visited := 0
		_ = walkBounded(ctx, dir, resolvedRoot, 4, 1500, &visited, func(path string, d fs.DirEntry, depth int) error {
			if d.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(d.Name()))
			if ext == ".xml" || ext == ".lua" {
				count++
			}
			return nil
		})
		details := fmt.Sprintf("%d files", count)
		if visited >= 1500 {
			details = fmt.Sprintf("%d+ files", count)
		}
		return ResourceResult{
			Path:    dir,
			State:   ResourceFound,
			Details: details,
		}
	}
	return ResourceResult{State: ResourceMissing, Details: "Not found"}
}

func detectServerProfile(root string, hasOTB bool) (string, string) {
	hasConfig := fsutil.FileExists(filepath.Join(root, "config.lua"))
	hasCanaryDir := fsutil.DirExists(filepath.Join(root, "data-otservbr-global")) ||
		fsutil.DirExists(filepath.Join(root, "data-canary")) ||
		fsutil.DirExists(filepath.Join(root, "data", "npclib"))
	if hasCanaryDir {
		return "Canary/Crystal", "Detected Canary/OTServBR directory layout"
	}
	hasTFSDirs := fsutil.DirExists(filepath.Join(root, "data", "XML")) ||
		fsutil.DirExists(filepath.Join(root, "data", "actions")) ||
		fsutil.DirExists(filepath.Join(root, "data", "creaturescripts")) ||
		fsutil.DirExists(filepath.Join(root, "data", "scripts"))
	if hasConfig && hasOTB && hasTFSDirs {
		return "TFS", "Detected config.lua, items.otb and TFS data layout"
	}
	if hasOTB && hasTFSDirs {
		return "TFS", "Detected items.otb and TFS data layout"
	}
	return "Unknown/Generic", "Standard layout not confirmed; resources can still be configured manually"
}

func sortDetectedMaps(maps []DetectedMap) {
	priority := func(relDir string) int {
		switch strings.ToLower(relDir) {
		case "data/world", "data/worlds":
			return 0
		case "data/map", "data/maps":
			return 1
		case "world", "worlds", "map", "maps":
			return 2
		case "":
			return 3
		default:
			return 4
		}
	}
	slices.SortFunc(maps, func(a, b DetectedMap) int {
		if pa, pb := priority(a.RelDir), priority(b.RelDir); pa != pb {
			return pa - pb
		}
		if a.Name != b.Name {
			return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		}
		return strings.Compare(strings.ToLower(a.Path), strings.ToLower(b.Path))
	})
}

func walkBounded(ctx context.Context, startDir, resolvedRoot string, maxDepth, maxEntries int, visited *int, fn func(path string, d fs.DirEntry, depth int) error) error {
	var walk func(dir string, depth int) error
	walk = func(dir string, depth int) error {
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		if depth > maxDepth || *visited >= maxEntries {
			return nil
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			// Skip unreadable directories without failing the whole scan.
			return nil
		}
		for _, entry := range entries {
			if ctx != nil && ctx.Err() != nil {
				return ctx.Err()
			}
			if *visited >= maxEntries {
				return nil
			}
			*visited++
			name := entry.Name()
			full := filepath.Join(dir, name)
			if entry.Type()&fs.ModeSymlink != 0 {
				target, err := filepath.EvalSymlinks(full)
				if err != nil || !isWithinRoot(target, resolvedRoot) {
					continue
				}
			}
			if entry.IsDir() {
				if ignoredScanDirs[strings.ToLower(name)] {
					continue
				}
				if err := walk(full, depth+1); err != nil {
					return err
				}
				continue
			}
			if err := fn(full, entry, depth); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(startDir, 0)
}

func isWithinRoot(target, root string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}
