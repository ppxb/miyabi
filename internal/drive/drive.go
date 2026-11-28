package drive

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/guangya"
	"github.com/ppxb/miyabi/internal/pan"
	"github.com/ppxb/miyabi/internal/syncx"
	"golang.org/x/sync/singleflight"
)

const (
	// Provider115 and ProviderGuangya identify the storage backend of an account.
	Provider115     = "115"
	ProviderGuangya = "guangya"
)

// Client abstracts the upstream 115 API calls.
type Client interface {
	Close()
	Account(context.Context, string) (pan.Account, error)
	BeginLogin(context.Context) (*pan.Login, error)
	LoginStatus(context.Context, *pan.Login) (pan.LoginState, error)
	ExchangeToken(context.Context, *pan.Login) (pan.Tokens, error)
	RefreshToken(context.Context, string) (pan.Tokens, error)
	List(context.Context, string, string, int, int) (pan.FilePage, error)
	Info(context.Context, string, string) (pan.FileInfo, error)
	ReadMetadata(context.Context, string, string, int64) ([]byte, error)
	AddOffline(context.Context, string, string, string) (string, error)
	RemoveOffline(context.Context, string, string) error
	OfflineTasks(context.Context, string, int) (pan.OfflinePage, error)
	PlayURL(context.Context, string, string, string) ([]pan.PlaySource, error)
	DownloadURL(context.Context, string, string, string) (string, error)
	OpenMedia(context.Context, string, string, http.Header) (*http.Response, error)
}

type mountRecord struct {
	AccountID string `json:"account_id"`
	Provider  string `json:"provider,omitempty"`
	domain.LibraryDirectory
}

type accountStatusCacheEntry struct {
	status   AccountStatus
	cachedAt time.Time
}

func (r mountRecord) source() domain.LibrarySource {
	return domain.LibrarySource{
		Provider:  r.Provider,
		AccountID: r.AccountID,
		Directory: r.LibraryDirectory,
	}
}

// Drive coordinates the registry of 115 accounts and their mounts. Every account
// is independent; there is no primary account.
type Drive struct {
	database  *ent.Client
	client    Client
	providers map[string]Client
	guangya   *guangya.Client

	mu     sync.Mutex
	commit syncx.ContextLock
	work   sync.WaitGroup
	closed bool

	login         qrLogin
	guangyaLogin  guangyaLogin
	mountListener MountListener
	media         mediaResolver

	// Account profiles come from the provider; reuse them and invalidate when
	// the stored account or mount changes.
	statusMu         sync.Mutex
	accountStatuses  map[string]accountStatusCacheEntry
	statusGeneration uint64
	now              func() time.Time

	// Credential/authorization versions are runtime invalidation counters for
	// in-flight sessions; account data itself lives in PanAccount rows.
	accountCredVersions map[string]uint64
	accountAuthVersions map[string]uint64
	accountRefresh      singleflight.Group
	refreshStop         chan struct{}
}

func New(ctx context.Context, db *ent.Client) (*Drive, error) {
	gy := guangya.New()
	return newDrive(ctx, db, map[string]Client{
		Provider115:     pan.New(),
		ProviderGuangya: gy,
	}, gy)
}

// NewWithClient keeps the 115-only constructor used by tests.
func NewWithClient(ctx context.Context, db *ent.Client, client Client) (*Drive, error) {
	return newDrive(ctx, db, map[string]Client{Provider115: client}, nil)
}

func newDrive(_ context.Context, db *ent.Client, providers map[string]Client, gy *guangya.Client) (*Drive, error) {
	d := &Drive{
		database:            db,
		client:              providers[Provider115],
		providers:           providers,
		guangya:             gy,
		refreshStop:         make(chan struct{}),
		accountCredVersions: make(map[string]uint64),
		accountAuthVersions: make(map[string]uint64),
		accountStatuses:     make(map[string]accountStatusCacheEntry),
		now:                 time.Now,
	}
	d.login.drive = d
	d.guangyaLogin.drive = d
	return d, nil
}

func (d *Drive) providerClient(provider string) (Client, bool) {
	if provider == "" {
		provider = Provider115
	}
	client, ok := d.providers[provider]
	return client, ok
}

func (d *Drive) Close() {
	d.mu.Lock()
	if !d.closed {
		d.closed = true
		if d.refreshStop != nil {
			close(d.refreshStop)
			d.refreshStop = nil
		}
	}
	d.mu.Unlock()
	d.work.Wait()
	if d.client != nil {
		d.client.Close()
	}
	if d.guangya != nil && Client(d.guangya) != d.client {
		d.guangya.Close()
	}
}

func (d *Drive) closedState() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.closed
}

// StartWork registers work that Close must wait for, such as a remote
// mutation that has to be recorded once started. It returns false once the
// drive is closed.
func (d *Drive) StartWork() (func(), bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, false
	}
	d.work.Add(1)
	return d.work.Done, true
}

// Commit executes a database transaction under the drive commit lock.
func (d *Drive) Commit(ctx context.Context, fn func(tx *ent.Tx) error) error {
	if err := d.commit.Lock(ctx); err != nil {
		return err
	}
	defer d.commit.Unlock()
	return ent.WithTx(ctx, d.database, fn)
}

// OpenMedia streams media from the default 115 client. Callers that may handle
// other providers should use OpenMediaFor.
func (d *Drive) OpenMedia(ctx context.Context, method, address string, headers http.Header) (*http.Response, error) {
	client, ok := d.providerClient(Provider115)
	if !ok {
		return nil, domain.E(domain.KindInvalid, "115 client is not configured", nil)
	}
	return client.OpenMedia(ctx, method, address, headers)
}

// OpenMediaFor streams media through the client that owns accountID.
func (d *Drive) OpenMediaFor(ctx context.Context, accountID, method, address string, headers http.Header) (*http.Response, error) {
	record, err := d.accountRecord(ctx, accountID)
	if err != nil {
		return nil, err
	}
	client, ok := d.providerClient(record.Provider)
	if !ok {
		return nil, domain.E(domain.KindInvalid, "存储账号的来源不受支持", nil)
	}
	return client.OpenMedia(ctx, method, address, headers)
}

// Source returns one mounted account's library source (the first by account id).
// It is a display/test convenience; no account is privileged.
func (d *Drive) Source() *domain.LibrarySource {
	mounts, err := d.Mounts(context.Background())
	if err != nil || len(mounts) == 0 {
		return nil
	}
	return &mounts[0]
}
