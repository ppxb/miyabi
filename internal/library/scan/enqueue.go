package scan

import (
	"context"
	"fmt"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/library/scrape"
	"github.com/ppxb/miyabi/internal/tasks"
)

// ScrapeJob describes one movie metadata job about to be enqueued.
type ScrapeJob struct {
	MovieID    int
	Code       string
	ManualCode string
	Rebuild    bool
	Source     domain.LibrarySource
	ScanTaskID int
}

// ActiveScrape is one queued or running scrape task that can still take work.
type ActiveScrape struct {
	TaskID     int
	ScanTaskID int
	Rebuild    bool
	Source     domain.LibrarySource
}

// ScrapeEnqueuePolicy decides what an active scrape task means for a new job.
type ScrapeEnqueuePolicy struct {
	// MatchSameSource only considers active jobs from the job's own source.
	MatchSameSource bool
	// RebuildOnly lets a rebuild job ignore active non-rebuild jobs.
	RebuildOnly bool
	// Reuse accepts the first active job that passed the filters above.
	Reuse bool
	// ReuseSameSourceRebuild accepts an active rebuild job from the same source;
	// every other active job is reported as a conflict.
	ReuseSameSourceRebuild bool
}

var (
	// ScanScrapePolicy reuses work already queued for the same mount. A rebuild
	// scan supersedes ordinary work that cannot pick up its files.
	ScanScrapePolicy = ScrapeEnqueuePolicy{MatchSameSource: true, RebuildOnly: true, Reuse: true}
	// LocalScrapePolicy skips a movie owned by any queued or running job.
	LocalScrapePolicy = ScrapeEnqueuePolicy{Reuse: true}
	// ManualScrapePolicy joins an active rebuild of the same source and reports
	// every other active job as a conflict.
	ManualScrapePolicy = ScrapeEnqueuePolicy{ReuseSameSourceRebuild: true}
	// ConflictScrapePolicy refuses every active job, for code corrections.
	ConflictScrapePolicy = ScrapeEnqueuePolicy{}
)

// ScrapePlan is the deduplicated result for one batch of jobs.
type ScrapePlan struct {
	Create []ScrapeJob
	Reused map[int]ActiveScrape // by movie id
}

// scrapeEnqueueBatchSize bounds both the resource-key lookup and the insert.
// SQLite binds at most 32766 variables; a task row uses eight variables, so a
// whole-library local scan must be chunked well below that limit.
const scrapeEnqueueBatchSize = 500

// activeScrapePayload is the subset of a stored scrape payload needed to
// deduplicate jobs. Decoding only these fields keeps unrelated checkpoint data
// (document, artwork, preview state) from invalidating an enqueue decision.
type activeScrapePayload struct {
	Rebuild    bool                 `json:"rebuild"`
	Source     domain.LibrarySource `json:"source"`
	ScanTaskID int                  `json:"scan_task_id"`
	Completed  bool                 `json:"completed"`
}

// PlanScrapes deduplicates a batch of scrape jobs against queued and running
// tasks. The caller must hold the queue lock and run inside tx so the decision
// cannot race another enqueue. A task that has reached its publication
// checkpoint cannot pick up files found later and is ignored.
func PlanScrapes(ctx context.Context, tx *ent.Tx, jobs []ScrapeJob, policy ScrapeEnqueuePolicy) (ScrapePlan, error) {
	plan := ScrapePlan{Reused: make(map[int]ActiveScrape, len(jobs))}
	if len(jobs) == 0 {
		return plan, nil
	}
	keys := make([]string, 0, len(jobs))
	seen := make(map[string]bool, len(jobs))
	for _, job := range jobs {
		key := resourceKey(job.MovieID)
		if seen[key] {
			continue
		}
		seen[key] = true
		keys = append(keys, key)
	}
	active := make(map[string][]ActiveScrape, len(keys))
	for start := 0; start < len(keys); start += scrapeEnqueueBatchSize {
		end := start + scrapeEnqueueBatchSize
		if end > len(keys) {
			end = len(keys)
		}
		records, err := tx.Task.Query().
			Where(task.TypeEQ(tasks.KindScrape.String()), task.ResourceKeyIn(keys[start:end]...),
				task.StatusIn(task.StatusQueued, task.StatusRunning)).
			Order(task.ByID()).
			Select(task.FieldID, task.FieldResourceKey, task.FieldPayload).
			All(ctx)
		if err != nil {
			return ScrapePlan{}, fmt.Errorf("find active scrape jobs: %w", err)
		}
		for _, record := range records {
			input, err := tasks.DecodePayload[activeScrapePayload](record.Payload)
			if err != nil {
				return ScrapePlan{}, fmt.Errorf("read active scrape job %d: %w", record.ID, err)
			}
			if input.Completed {
				// Publication checkpoint reached; the task can no longer pick up files
				// discovered after it, so a new job must be queued instead.
				continue
			}
			active[record.ResourceKey] = append(active[record.ResourceKey], ActiveScrape{
				TaskID: record.ID, ScanTaskID: input.ScanTaskID,
				Rebuild: input.Rebuild, Source: input.Source,
			})
		}
	}
	for _, job := range jobs {
		item, found := firstActive(active[resourceKey(job.MovieID)], job, policy)
		if !found {
			plan.Create = append(plan.Create, job)
			continue
		}
		reusable := policy.Reuse ||
			(policy.ReuseSameSourceRebuild && job.Rebuild && item.Rebuild && item.Source == job.Source)
		if !reusable {
			return ScrapePlan{}, domain.E(domain.KindConflict, "该影片正在处理中，请完成后再操作", nil)
		}
		plan.Reused[job.MovieID] = item
	}
	return plan, nil
}

// NewScrapeTask builds one scrape task row. The caller creates it inside the
// same transaction that produced the plan.
func NewScrapeTask(tx *ent.Tx, job ScrapeJob) (*ent.TaskCreate, error) {
	input := scrape.MetadataPayload{
		Rebuild: job.Rebuild, Source: job.Source, ScanTaskID: job.ScanTaskID,
		MovieID: job.MovieID, Code: job.Code, ManualCode: job.ManualCode,
	}
	encoded, err := tasks.EncodePayload(input)
	if err != nil {
		return nil, err
	}
	return tx.Task.Create().SetType(tasks.KindScrape.String()).
		SetResourceKey(resourceKey(job.MovieID)).SetPayload(encoded), nil
}

// CreateScrapeTasks inserts planned jobs in bounded batches. A single
// CreateBulk binds eight variables per row, so a whole-library local scan must
// be chunked to stay below SQLite's bind-variable limit.
func CreateScrapeTasks(ctx context.Context, tx *ent.Tx, jobs []ScrapeJob) error {
	for start := 0; start < len(jobs); start += scrapeEnqueueBatchSize {
		end := start + scrapeEnqueueBatchSize
		if end > len(jobs) {
			end = len(jobs)
		}
		builders := make([]*ent.TaskCreate, 0, end-start)
		for _, job := range jobs[start:end] {
			create, err := NewScrapeTask(tx, job)
			if err != nil {
				return err
			}
			builders = append(builders, create)
		}
		if err := tx.Task.CreateBulk(builders...).Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}

func resourceKey(movieID int) string {
	return fmt.Sprintf("movie:%d", movieID)
}

// sameMount matches the stored source identity used by scan reconciliation:
// account plus directory id, ignoring display name and path.
func sameMount(a, b domain.LibrarySource) bool {
	return a.AccountID == b.AccountID && a.Directory.ID == b.Directory.ID
}

func firstActive(items []ActiveScrape, job ScrapeJob, policy ScrapeEnqueuePolicy) (ActiveScrape, bool) {
	for _, item := range items {
		if policy.MatchSameSource && !sameMount(item.Source, job.Source) {
			continue
		}
		if policy.RebuildOnly && job.Rebuild && !item.Rebuild {
			continue
		}
		return item, true
	}
	return ActiveScrape{}, false
}
