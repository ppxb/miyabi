package offline

import (
	"strings"
	"testing"
)

func TestPendingSignalsAfterSubmissionAndControl(t *testing.T) {
	r := newRecoveryRig(t)
	record := r.add(offlineHashA)
	awaitPending(t, r.s, "submission")

	if _, err := r.s.Cancel(r.t.Context(), record.ID); err != nil {
		t.Fatal(err)
	}
	awaitPending(t, r.s, "cancel")
}

func TestFailedSubmissionDoesNotSignalPendingSync(t *testing.T) {
	service, _ := offlineAddFixture(t)
	if _, err := service.AddForAccount(t.Context(), "100", "fixture-movie", strings.Repeat("c", 40)); err == nil {
		t.Fatal("expected unknown magnet to fail")
	}
	select {
	case <-service.Pending():
		t.Fatal("failed submission signalled the sync loop")
	default:
	}
}

func awaitPending(t *testing.T, service *Service, action string) {
	t.Helper()
	select {
	case <-service.Pending():
	default:
		t.Fatalf("%s did not signal the sync loop", action)
	}
}
