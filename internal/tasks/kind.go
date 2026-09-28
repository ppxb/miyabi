package tasks

// Kind identifies the type of background task.
type Kind string

const (
	KindScan              Kind = "scan"
	KindScrape            Kind = "scrape"
	KindCover             Kind = "cover"
	KindSubscriptionBatch Kind = "subscription_batch"
)

// String returns the string representation of the task kind.
func (k Kind) String() string {
	return string(k)
}
