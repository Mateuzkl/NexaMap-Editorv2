package app

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nekiro/ots-creator/internal/settings"
)

type WorkspaceService struct {
	session  *Session
	settings *settings.Store
	gen      atomic.Int64
	mu       sync.Mutex
	cancel   context.CancelFunc
}

func NewWorkspaceService(session *Session, store *settings.Store) *WorkspaceService {
	return &WorkspaceService{session: session, settings: store}
}

func (ws *WorkspaceService) nextScanContext() (int64, context.Context, context.CancelFunc) {
	gen := ws.gen.Add(1)
	ws.mu.Lock()
	if ws.cancel != nil {
		ws.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	ws.cancel = cancel
	ws.mu.Unlock()
	return gen, ctx, cancel
}

func (ws *WorkspaceService) CancelScan() {
	ws.mu.Lock()
	if ws.cancel != nil {
		ws.cancel()
		ws.cancel = nil
	}
	ws.mu.Unlock()
}

func (ws *WorkspaceService) InspectWorkspace(clientRoot, serverRoot, selectedClientPath, selectedOtbPath, selectedXmlPath string) WorkspaceInspection {
	gen, ctx, cancel := ws.nextScanContext()
	defer cancel()

	var remembered func(string) string
	if ws.session != nil {
		remembered = ws.session.rememberedOTB
	}
	client := inspectClientWorkspace(clientRoot, selectedClientPath, remembered)
	server := scanServerRoot(ctx, serverRoot, selectedOtbPath, selectedXmlPath)
	return WorkspaceInspection{
		Generation: gen,
		Client:     client,
		Server:     server,
	}
}

func (ws *WorkspaceService) ScanServerRoot(serverRoot string) ServerResources {
	_, ctx, cancel := ws.nextScanContext()
	defer cancel()
	return scanServerRoot(ctx, serverRoot, "", "")
}

func (ws *WorkspaceService) ListWorkspaceMaps(serverRoot string) []DetectedMap {
	return ws.ScanServerRoot(serverRoot).Maps
}

func (ws *WorkspaceService) SaveWorkspace(profile settings.WorkspaceProfile) ([]settings.WorkspaceProfile, error) {
	if ws.settings == nil {
		return []settings.WorkspaceProfile{}, nil
	}
	return ws.settings.SaveWorkspace(profile)
}

func (ws *WorkspaceService) RenameWorkspace(id, name string) ([]settings.WorkspaceProfile, error) {
	if ws.settings == nil {
		return []settings.WorkspaceProfile{}, nil
	}
	return ws.settings.RenameWorkspace(id, name)
}

func (ws *WorkspaceService) RemoveWorkspace(id string) ([]settings.WorkspaceProfile, error) {
	if ws.settings == nil {
		return []settings.WorkspaceProfile{}, nil
	}
	return ws.settings.RemoveWorkspace(id)
}

func (ws *WorkspaceService) SetActiveWorkspace(id string) error {
	if ws.settings == nil {
		return nil
	}
	return ws.settings.SetActiveWorkspace(id)
}
