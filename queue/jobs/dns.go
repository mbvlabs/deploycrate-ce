package jobs

import (
	"github.com/google/uuid"
	"github.com/riverqueue/river"
)

const DNSQueue = "dns_reconciliation"

type DNSReconciliationArgs struct {
	BindingID  uuid.UUID `json:"binding_id" river:"unique"`
	Generation int64     `json:"generation" river:"unique"`
}

func (DNSReconciliationArgs) Kind() string { return "dns_reconciliation" }

func (args DNSReconciliationArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: DNSQueue, MaxAttempts: 8,
		UniqueOpts: river.UniqueOpts{ByArgs: true},
		Tags:       []string{"dns", "cloudflare"},
	}
}

func DNSReconciliationInsertOpts(bindingID uuid.UUID, generation int64) *river.InsertOpts {
	opts := DNSReconciliationArgs{BindingID: bindingID, Generation: generation}.InsertOpts()
	return &opts
}
