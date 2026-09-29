package emby

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/export"
	"github.com/ppxb/miyabi/internal/gfriends"
)

// Service manages communication, configuration persistence, and batch notification to Emby.
type Service struct {
	db                *ent.Client
	mu                sync.RWMutex
	cfg               Config
	defaultPublicURL  string
	client            *embyClient
	actors            *actorSync
	wakeNotifications chan struct{}
	ctx               context.Context
	cancel            context.CancelFunc
	wg                sync.WaitGroup
	exportMgr         *export.Manager
	updateMu          sync.Mutex
	scheduleLocalScan func(context.Context) error
}

type Dependencies struct {
	GFriends          *gfriends.Client
	Media             MediaFetcher
	ExportManager     *export.Manager
	ScheduleLocalScan func(context.Context) error
}

// NewService restores configuration and installs dependencies before starting workers.
func NewService(ctx context.Context, db *ent.Client, initial Config, deps Dependencies) (*Service, error) {
	loaded, found, err := database.LoadSetting[Config](ctx, db, SettingKey)
	if err != nil {
		return nil, fmt.Errorf("load emby setting: %w", err)
	}

	cfg := initial
	if found {
		cfg = loaded
		if cfg.LocalDir == "" {
			cfg.LocalDir = initial.LocalDir
		}
		if cfg.PublicURL == "" && initial.PublicURL != "" {
			cfg.PublicURL = initial.PublicURL
		}
	}

	subCtx, cancel := context.WithCancel(context.Background())
	s := &Service{
		db:                db,
		cfg:               cfg,
		defaultPublicURL:  initial.PublicURL,
		client:            newEmbyClient(),
		wakeNotifications: make(chan struct{}, 1),
		ctx:               subCtx,
		cancel:            cancel,
		exportMgr:         deps.ExportManager,
		scheduleLocalScan: deps.ScheduleLocalScan,
	}
	s.actors = newActorSync(db, s.client, s.currentConfig, deps.GFriends, deps.Media)
	if s.exportMgr != nil {
		cfgExport := s.exportMgr.Config()
		cfgExport.EmbyDir, cfgExport.PublicURL = cfg.LocalDir, cfg.PublicURL
		s.exportMgr.Set(cfgExport)
	}

	s.wg.Add(1)
	go s.worker(subCtx)

	if s.cfg.Enabled && s.cfg.IsSyncActors() {
		s.actors.schedule(10 * time.Second)
	}

	return s, nil
}

// Close makes a final delivery attempt; outstanding updates remain in the database.
func (s *Service) Close() {
	s.cancel()
	s.actors.close()
	s.wg.Wait()
}

// Config returns the current active configuration.
func (s *Service) Config(context.Context) (Config, error) { return s.currentConfig(), nil }

func (s *Service) currentConfig() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cfg := s.cfg
	if cfg.PublicURL == "" && s.defaultPublicURL != "" {
		cfg.PublicURL = s.defaultPublicURL
	}
	return cfg
}

// UpdateConfig validates and persists the new configuration to the database.
func (s *Service) UpdateConfig(ctx context.Context, cfg Config) error {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	s.mu.Lock()
	if cfg.LocalDir == "" {
		cfg.LocalDir = s.cfg.LocalDir
	}
	if cfg.PublicURL == "" {
		cfg.PublicURL = s.defaultPublicURL
	}
	oldPublicURL := s.cfg.PublicURL
	oldLocalDir := s.cfg.LocalDir
	token := s.exportMgr.Config().STRMToken
	s.mu.Unlock()

	if err := cfg.Normalize(); err != nil {
		return err
	}

	if err := database.SaveSetting(ctx, s.db, SettingKey, cfg); err != nil {
		return fmt.Errorf("save emby setting: %w", err)
	}

	s.mu.Lock()
	s.cfg = cfg
	exportMgr := s.exportMgr
	s.mu.Unlock()
	s.actors.reconfigure()

	if exportMgr != nil && cfg.LocalDir != "" {
		exportMgr.Set(export.Config{
			EmbyDir:   cfg.LocalDir,
			PublicURL: cfg.PublicURL,
			STRMToken: token,
		})
	}
	if exportMgr != nil && (oldPublicURL != cfg.PublicURL || oldLocalDir != cfg.LocalDir) {
		s.startSTRMRewrite(exportMgr)
	}

	if cfg.Enabled && cfg.IsSyncActors() {
		s.actors.schedule(2 * time.Second)
	}

	if s.scheduleLocalScan != nil {
		if err := s.scheduleLocalScan(ctx); err != nil {
			return domain.E(domain.KindBusy, "Emby 设置已保存，但本地扫描入队失败，请重新保存重试", err)
		}
	}
	return s.RetryPending(ctx)
}

// StartStartupSTRMRewrite uses the shared manager's current configuration.
func (s *Service) StartStartupSTRMRewrite() {
	s.mu.RLock()
	mgr := s.exportMgr
	s.mu.RUnlock()
	if mgr != nil {
		s.startSTRMRewrite(mgr)
	}
}

func (s *Service) startSTRMRewrite(mgr *export.Manager) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		count, err := mgr.RewriteSTRM(s.ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("failed to rewrite STRM files", "error", err)
		} else if count > 0 {
			slog.Info("rewrote STRM files", "count", count)
			if err := s.NotifyUpdated(s.ctx, mgr.Config().EmbyDir); err != nil {
				slog.Error("persist Emby notification", "error", err)
			}
		}
	}()
}

// Test validates connection parameters by querying /System/Info.
func (s *Service) Test(ctx context.Context, cfg Config) (ServerInfo, error) {
	serverURL := strings.TrimRight(strings.TrimSpace(cfg.ServerURL), "/")
	apiKey := strings.TrimSpace(cfg.APIKey)
	if serverURL == "" || apiKey == "" {
		return ServerInfo{}, domain.E(domain.KindInvalid, "请先填写 Emby 服务器地址与 API Key", nil)
	}
	return s.client.ping(ctx, Config{ServerURL: serverURL, APIKey: apiKey})
}

func (s *Service) sendBatch(ctx context.Context, cfg Config, localPaths []string) error {

	if !cfg.ready() || len(localPaths) == 0 {
		return nil
	}

	updates := make([]mediaUpdateItem, 0, len(localPaths))
	for _, lp := range localPaths {
		embyPath := translatePath(lp, cfg.LocalDir, cfg.MediaPath)
		if embyPath != "" {
			updateType := "Created"
			if _, err := os.Stat(lp); os.IsNotExist(err) {
				updateType = "Deleted"
			}
			updates = append(updates, mediaUpdateItem{
				Path:       embyPath,
				UpdateType: updateType,
			})
		}
	}

	if len(updates) == 0 {
		return nil
	}

	if err := s.client.notify(ctx, cfg, updates); err != nil {
		return err
	}

	if err := s.client.refresh(ctx, cfg); err != nil {
		slog.WarnContext(ctx, "failed to trigger emby library refresh", "error", err)
	}

	return nil
}

func translatePath(localPath, localDir, mediaPath string) string {
	localPath = filepath.Clean(localPath)
	if mediaPath == "" {
		if !filepath.IsAbs(localPath) && !strings.HasPrefix(localPath, "/") && !strings.HasPrefix(localPath, "\\") {
			if abs, err := filepath.Abs(localPath); err == nil {
				localPath = abs
			}
		}
		if filepath.IsAbs(localPath) && filepath.VolumeName(localPath) != "" {
			return filepath.Clean(localPath)
		}
		return filepath.ToSlash(localPath)
	}

	mediaPath = strings.TrimRight(filepath.ToSlash(mediaPath), "/")
	if localDir != "" {
		cleanLocalDir := filepath.Clean(localDir)
		rel, err := filepath.Rel(cleanLocalDir, localPath)
		if err == nil {
			slashRel := filepath.ToSlash(rel)
			if slashRel != ".." && !strings.HasPrefix(slashRel, "../") {
				return path.Join(mediaPath, slashRel)
			}
		}
		if absLocalDir, err1 := filepath.Abs(cleanLocalDir); err1 == nil {
			if absLocalPath, err2 := filepath.Abs(localPath); err2 == nil {
				if rel2, err3 := filepath.Rel(absLocalDir, absLocalPath); err3 == nil {
					slashRel2 := filepath.ToSlash(rel2)
					if slashRel2 != ".." && !strings.HasPrefix(slashRel2, "../") {
						return path.Join(mediaPath, slashRel2)
					}
				}
			}
		}
	}

	return ""
}
