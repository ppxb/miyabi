package scan

import (
	"fmt"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/library/scrape"
	"github.com/ppxb/miyabi/internal/tasks"
)

var (
	enqueueSourceA = domain.LibrarySource{AccountID: "account-a", Directory: domain.LibraryDirectory{ID: "root-a", Path: "/A"}}
	enqueueSourceB = domain.LibrarySource{AccountID: "account-b", Directory: domain.LibraryDirectory{ID: "root-b", Path: "/B"}}
)

func enqueueStore(t *testing.T) *ent.Client {
	t.Helper()
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store.Client
}

func createScrapeTask(t *testing.T, client *ent.Client, movieID int, status task.Status, input scrape.Payload) *ent.Task {
	t.Helper()
	body, err := tasks.EncodePayload(input)
	if err != nil {
		t.Fatal(err)
	}
	return client.Task.Create().SetType(tasks.KindScrape.String()).
		SetResourceKey(fmt.Sprintf("movie:%d", movieID)).SetStatus(status).SetPayload(body).SaveX(t.Context())
}

func runPlan(t *testing.T, client *ent.Client, jobs []ScrapeJob, policy ScrapeEnqueuePolicy) (ScrapePlan, error) {
	t.Helper()
	var plan ScrapePlan
	err := ent.WithTx(t.Context(), client, func(tx *ent.Tx) error {
		var err error
		plan, err = PlanScrapes(t.Context(), tx, jobs, policy)
		return err
	})
	return plan, err
}

func TestPlanScrapesIgnoresPublishedCheckpoint(t *testing.T) {
	client := enqueueStore(t)
	createScrapeTask(t, client, 1, task.StatusQueued, scrape.Payload{
		MetadataPayload: scrape.MetadataPayload{Source: enqueueSourceA, MovieID: 1, ScanTaskID: 9},
		Completed:       true,
	})
	for name, policy := range map[string]ScrapeEnqueuePolicy{
		"local": LocalScrapePolicy,
		"scan":  ScanScrapePolicy,
	} {
		t.Run(name, func(t *testing.T) {
			plan, err := runPlan(t, client, []ScrapeJob{{MovieID: 1, Source: enqueueSourceA}}, policy)
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Create) != 1 || len(plan.Reused) != 0 {
				t.Fatalf("published task blocked new work: %+v", plan)
			}
		})
	}
}

func TestPlanScrapesLocalReusesAnySource(t *testing.T) {
	client := enqueueStore(t)
	active := createScrapeTask(t, client, 2, task.StatusRunning, scrape.Payload{
		MetadataPayload: scrape.MetadataPayload{Source: enqueueSourceB, MovieID: 2, ScanTaskID: 12},
	})
	plan, err := runPlan(t, client, []ScrapeJob{{MovieID: 2, Source: enqueueSourceA}}, LocalScrapePolicy)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Create) != 0 || plan.Reused[2].TaskID != active.ID {
		t.Fatalf("local scan did not reuse cross-source work: %+v", plan)
	}
}

func TestPlanScrapesScanPolicyMatchesSourceAndRebuild(t *testing.T) {
	client := enqueueStore(t)
	active := createScrapeTask(t, client, 20, task.StatusQueued, scrape.Payload{
		MetadataPayload: scrape.MetadataPayload{Source: enqueueSourceA, MovieID: 20, ScanTaskID: 42},
	})
	for name, tc := range map[string]struct {
		job    ScrapeJob
		create bool
	}{
		"same source": {job: ScrapeJob{MovieID: 20, Source: enqueueSourceA}},
		"same mount renamed": {job: ScrapeJob{MovieID: 20, Source: domain.LibrarySource{
			AccountID: enqueueSourceA.AccountID,
			Directory: domain.LibraryDirectory{ID: enqueueSourceA.Directory.ID, Name: "renamed", Path: "/renamed"},
		}}},
		"other source":         {job: ScrapeJob{MovieID: 20, Source: enqueueSourceB}, create: true},
		"rebuild supersedes":   {job: ScrapeJob{MovieID: 20, Source: enqueueSourceA, Rebuild: true}, create: true},
		"rebuild other source": {job: ScrapeJob{MovieID: 20, Source: enqueueSourceB, Rebuild: true}, create: true},
	} {
		t.Run(name, func(t *testing.T) {
			plan, err := runPlan(t, client, []ScrapeJob{tc.job}, ScanScrapePolicy)
			if err != nil {
				t.Fatal(err)
			}
			if tc.create != (len(plan.Create) == 1) || (!tc.create && plan.Reused[20].TaskID != active.ID) {
				t.Fatalf("create=%v plan=%+v", tc.create, plan)
			}
		})
	}
	// A rebuild job must find a rebuild task even when an older ordinary task
	// exists for the same movie.
	ordinary := createScrapeTask(t, client, 21, task.StatusQueued, scrape.Payload{
		MetadataPayload: scrape.MetadataPayload{Source: enqueueSourceA, MovieID: 21, ScanTaskID: 7},
	})
	rebuild := createScrapeTask(t, client, 21, task.StatusQueued, scrape.Payload{
		MetadataPayload: scrape.MetadataPayload{Source: enqueueSourceA, MovieID: 21, ScanTaskID: 8, Rebuild: true},
	})
	plan, err := runPlan(t, client, []ScrapeJob{{MovieID: 21, Source: enqueueSourceA, Rebuild: true}}, ScanScrapePolicy)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Create) != 0 || plan.Reused[21].TaskID != rebuild.ID || plan.Reused[21].TaskID == ordinary.ID ||
		plan.Reused[21].ScanTaskID != 8 {
		t.Fatalf("rebuild did not join the rebuild task: %+v", plan)
	}
	// An ordinary scan may reuse an active rebuild job.
	rebuildOnly := createScrapeTask(t, client, 22, task.StatusQueued, scrape.Payload{
		MetadataPayload: scrape.MetadataPayload{Source: enqueueSourceA, MovieID: 22, ScanTaskID: 6, Rebuild: true},
	})
	plan, err = runPlan(t, client, []ScrapeJob{{MovieID: 22, Source: enqueueSourceA}}, ScanScrapePolicy)
	if err != nil || len(plan.Create) != 0 || plan.Reused[22].TaskID != rebuildOnly.ID {
		t.Fatalf("ordinary scan did not reuse rebuild work: %+v %v", plan, err)
	}
}

func TestPlanScrapesManualPolicyConflictsUnlessSameSourceRebuild(t *testing.T) {
	client := enqueueStore(t)
	createScrapeTask(t, client, 30, task.StatusQueued, scrape.Payload{
		MetadataPayload: scrape.MetadataPayload{Source: enqueueSourceA, MovieID: 30, ScanTaskID: 1},
	})
	if _, err := runPlan(t, client, []ScrapeJob{{MovieID: 30, Source: enqueueSourceA, Rebuild: true}}, ManualScrapePolicy); !domain.IsKind(err, domain.KindConflict) {
		t.Fatalf("ordinary active job did not conflict: %v", err)
	}

	active := createScrapeTask(t, client, 31, task.StatusRunning, scrape.Payload{
		MetadataPayload: scrape.MetadataPayload{Source: enqueueSourceA, MovieID: 31, ScanTaskID: 77, Rebuild: true},
	})
	plan, err := runPlan(t, client, []ScrapeJob{{MovieID: 31, Source: enqueueSourceA, Rebuild: true}}, ManualScrapePolicy)
	if err != nil || len(plan.Create) != 0 || plan.Reused[31].ScanTaskID != 77 || plan.Reused[31].TaskID != active.ID {
		t.Fatalf("manual job did not join its rebuild: %+v %v", plan, err)
	}
	if _, err := runPlan(t, client, []ScrapeJob{{MovieID: 31, Source: enqueueSourceB, Rebuild: true}}, ManualScrapePolicy); !domain.IsKind(err, domain.KindConflict) {
		t.Fatalf("other source rebuild did not conflict: %v", err)
	}
	if _, err := runPlan(t, client, []ScrapeJob{{MovieID: 31, Source: enqueueSourceA, Rebuild: true}}, ConflictScrapePolicy); !domain.IsKind(err, domain.KindConflict) {
		t.Fatalf("correction did not conflict: %v", err)
	}
	// A published task no longer owns the movie, so a correction may enqueue.
	createScrapeTask(t, client, 32, task.StatusQueued, scrape.Payload{
		MetadataPayload: scrape.MetadataPayload{Source: enqueueSourceA, MovieID: 32, ScanTaskID: 3, Rebuild: true},
		Completed:       true,
	})
	plan, err = runPlan(t, client, []ScrapeJob{{MovieID: 32, Source: enqueueSourceA, Rebuild: true}}, ConflictScrapePolicy)
	if err != nil || len(plan.Create) != 1 || len(plan.Reused) != 0 {
		t.Fatalf("published task blocked a correction: %+v %v", plan, err)
	}
}

func TestNewScrapeTaskEncodesPayload(t *testing.T) {
	client := enqueueStore(t)
	job := ScrapeJob{
		MovieID: 40, Code: "ABP-001", ManualCode: "ABP-001", Rebuild: true,
		Source: enqueueSourceA, ScanTaskID: 55,
	}
	var created *ent.Task
	err := ent.WithTx(t.Context(), client, func(tx *ent.Tx) error {
		create, err := NewScrapeTask(tx, job)
		if err != nil {
			return err
		}
		created, err = create.Save(t.Context())
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	input, err := tasks.DecodePayload[scrape.Payload](created.Payload)
	if err != nil || created.Type != tasks.KindScrape.String() || created.ResourceKey != "movie:40" ||
		input.MovieID != 40 || input.ScanTaskID != 55 || !input.Rebuild || input.Source != enqueueSourceA ||
		input.Code != "ABP-001" || input.ManualCode != "ABP-001" {
		t.Fatalf("task=%+v payload=%+v err=%v", created, input, err)
	}
}

func TestPlanScrapesDecodesOnlyIdentityFields(t *testing.T) {
	client := enqueueStore(t)
	body, err := tasks.EncodePayload(map[string]any{
		"source": enqueueSourceA, "scan_task_id": 5, "rebuild": true, "completed": false,
		"document": "not a movie object", "artwork": 12,
	})
	if err != nil {
		t.Fatal(err)
	}
	active := client.Task.Create().SetType(tasks.KindScrape.String()).
		SetResourceKey("movie:50").SetPayload(body).SaveX(t.Context())
	plan, err := runPlan(t, client, []ScrapeJob{{MovieID: 50, Source: enqueueSourceA, Rebuild: true}}, ScanScrapePolicy)
	if err != nil || len(plan.Create) != 0 || plan.Reused[50].TaskID != active.ID {
		t.Fatalf("unrelated payload fields blocked reuse: %+v %v", plan, err)
	}
}

func TestPlanScrapesQueriesLargeBatches(t *testing.T) {
	client := enqueueStore(t)
	first := createScrapeTask(t, client, 1, task.StatusQueued, scrape.Payload{
		MetadataPayload: scrape.MetadataPayload{Source: enqueueSourceA, MovieID: 1, ScanTaskID: 11},
	})
	second := createScrapeTask(t, client, 550, task.StatusRunning, scrape.Payload{
		MetadataPayload: scrape.MetadataPayload{Source: enqueueSourceA, MovieID: 550, ScanTaskID: 12},
	})
	jobs := make([]ScrapeJob, 0, 600)
	for id := 1; id <= 600; id++ {
		jobs = append(jobs, ScrapeJob{MovieID: id, Source: enqueueSourceA})
	}
	plan, err := runPlan(t, client, jobs, ScanScrapePolicy)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Create) != len(jobs)-2 || plan.Reused[1].TaskID != first.ID || plan.Reused[550].TaskID != second.ID {
		t.Fatalf("create=%d reused=%d", len(plan.Create), len(plan.Reused))
	}
}

func TestCreateScrapeTasksBatchesLargeInput(t *testing.T) {
	client := enqueueStore(t)
	// One CreateBulk would bind 8 variables per row; 4097 rows exceed SQLite's
	// 32766-variable limit, so the helper must split the insert.
	jobs := make([]ScrapeJob, 4097)
	for i := range jobs {
		jobs[i] = ScrapeJob{MovieID: i + 1, Code: fmt.Sprintf("TEST-%04d", i), Source: enqueueSourceA}
	}
	err := ent.WithTx(t.Context(), client, func(tx *ent.Tx) error {
		return CreateScrapeTasks(t.Context(), tx, jobs)
	})
	if err != nil {
		t.Fatal(err)
	}
	got := client.Task.Query().Where(task.TypeEQ(tasks.KindScrape.String())).CountX(t.Context())
	if got != len(jobs) {
		t.Fatalf("scrape tasks=%d want=%d", got, len(jobs))
	}
}
