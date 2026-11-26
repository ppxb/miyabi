package tasks

import "testing"

func TestWakePoolFiltersByKind(t *testing.T) {
	bus := NewBus()
	scan, stopScan := bus.SubscribePool(KindScan)
	defer stopScan()
	scrape, stopScrape := bus.SubscribePool(KindScrape)
	defer stopScrape()
	all, stopAll := bus.SubscribePool()
	defer stopAll()

	bus.WakePool(KindScan)
	if len(scan) != 1 || len(all) != 1 {
		t.Fatalf("scan wake: scan=%d all=%d", len(scan), len(all))
	}
	if len(scrape) != 0 {
		t.Fatal("scan wake reached the scrape worker")
	}
	<-scan
	<-all

	bus.WakePool()
	if len(scan) != 1 || len(scrape) != 1 || len(all) != 1 {
		t.Fatalf("unfiltered wake: scan=%d scrape=%d all=%d", len(scan), len(scrape), len(all))
	}
}

func TestWakePoolIgnoresUnrelatedKinds(t *testing.T) {
	bus := NewBus()
	scan, stop := bus.SubscribePool(KindScan)
	defer stop()

	bus.WakePool(KindScrape)
	if len(scan) != 0 {
		t.Fatal("unrelated kind woke the scan worker")
	}
}
