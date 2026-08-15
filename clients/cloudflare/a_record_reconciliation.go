package cloudflare

import (
	"context"
	"sort"
	"strings"
)

type AddressRecordClient interface {
	ListAddressRecords(context.Context, string, string, string) ([]DNSRecord, error)
	CreateARecord(context.Context, string, string, DNSRecordInput) (DNSRecord, error)
	UpdateARecord(context.Context, string, string, string, DNSRecordInput) (DNSRecord, error)
	DeleteRecord(context.Context, string, string, string) error
}

type ARecordReconciliationInput struct {
	Token            string
	ZoneID           string
	Hostname         string
	DesiredIPv4      []string
	OwnershipMarker  string
	TrackedRecordIDs []string
	AdoptUnmanaged   bool
}

type AddressRecordClassification struct {
	Tracked      []DNSRecord
	CommentOwned []DNSRecord
	Unmanaged    []DNSRecord
}

type ARecordReconciliationResult struct {
	Classification     AddressRecordClassification
	Applied            []DNSRecord
	BlockedByUnmanaged bool
}

func ReconcileARecords(
	ctx context.Context,
	client AddressRecordClient,
	input ARecordReconciliationInput,
) (ARecordReconciliationResult, error) {
	remote, err := client.ListAddressRecords(ctx, input.Token, input.ZoneID, input.Hostname)
	if err != nil {
		return ARecordReconciliationResult{}, err
	}

	trackedIDs := make(map[string]struct{}, len(input.TrackedRecordIDs))
	for _, id := range input.TrackedRecordIDs {
		trackedIDs[id] = struct{}{}
	}
	classification := classifyAddressRecords(remote, trackedIDs, input.OwnershipMarker)
	result := ARecordReconciliationResult{Classification: classification}
	if len(classification.Unmanaged) > 0 && !input.AdoptUnmanaged {
		result.BlockedByUnmanaged = true
		return result, nil
	}

	owned := make([]DNSRecord, 0, len(remote))
	owned = append(owned, classification.Tracked...)
	owned = append(owned, classification.CommentOwned...)
	if input.AdoptUnmanaged {
		owned = append(owned, classification.Unmanaged...)
	}
	sortDNSRecords(owned)

	desired := append([]string(nil), input.DesiredIPv4...)
	sort.Strings(desired)
	result.Applied = make([]DNSRecord, 0, len(desired))
	for index, address := range desired {
		recordInput := DNSRecordInput{
			Type:    "A",
			Name:    input.Hostname,
			Content: address,
			TTL:     1,
			Proxied: true,
			Comment: input.OwnershipMarker,
		}
		var record DNSRecord
		if index < len(owned) && strings.EqualFold(owned[index].Type, "A") {
			record, err = client.UpdateARecord(
				ctx,
				input.Token,
				input.ZoneID,
				owned[index].ID,
				recordInput,
			)
		} else {
			if index < len(owned) {
				err = client.DeleteRecord(ctx, input.Token, input.ZoneID, owned[index].ID)
			}
			if err == nil {
				record, err = client.CreateARecord(ctx, input.Token, input.ZoneID, recordInput)
			}
		}
		if err != nil {
			return ARecordReconciliationResult{}, err
		}
		result.Applied = append(result.Applied, record)
	}
	for index := len(desired); index < len(owned); index++ {
		if err := client.DeleteRecord(ctx, input.Token, input.ZoneID, owned[index].ID); err != nil {
			return ARecordReconciliationResult{}, err
		}
	}
	return result, nil
}

func classifyAddressRecords(
	records []DNSRecord,
	trackedIDs map[string]struct{},
	marker string,
) AddressRecordClassification {
	classification := AddressRecordClassification{
		Tracked:      make([]DNSRecord, 0),
		CommentOwned: make([]DNSRecord, 0),
		Unmanaged:    make([]DNSRecord, 0),
	}
	for _, record := range records {
		if _, tracked := trackedIDs[record.ID]; tracked {
			classification.Tracked = append(classification.Tracked, record)
		} else if record.Comment == marker {
			classification.CommentOwned = append(classification.CommentOwned, record)
		} else {
			classification.Unmanaged = append(classification.Unmanaged, record)
		}
	}
	sortDNSRecords(classification.Tracked)
	sortDNSRecords(classification.CommentOwned)
	sortDNSRecords(classification.Unmanaged)
	return classification
}

func sortDNSRecords(records []DNSRecord) {
	sort.Slice(records, func(i, j int) bool {
		if records[i].ID != records[j].ID {
			return records[i].ID < records[j].ID
		}
		if records[i].Type != records[j].Type {
			return records[i].Type < records[j].Type
		}
		if records[i].Name != records[j].Name {
			return records[i].Name < records[j].Name
		}
		return records[i].Content < records[j].Content
	})
}
