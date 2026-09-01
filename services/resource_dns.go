package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	cloudflareclient "deploycrate-ce/clients/cloudflare"
	"deploycrate-ce/internal/secretcrypto"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"

	"github.com/google/uuid"
)

type ResourceDNS struct {
	db          storage.Pool
	connections *DNSConnections
	client      CloudflareDNSClient
}

type ResourceDNSStatus struct {
	Mode           string            `json:"mode"`
	ZoneID         string            `json:"zoneId"`
	ZoneName       string            `json:"zoneName"`
	ConnectionName string            `json:"connectionName"`
	State          string            `json:"state"`
	LastError      string            `json:"lastError"`
	AppliedAt      string            `json:"appliedAt"`
	Records        []DNSRecordStatus `json:"records"`
}

func NewResourceDNS(
	db storage.Pool,
	connections *DNSConnections,
	client CloudflareDNSClient,
) *ResourceDNS {
	return &ResourceDNS{db: db, connections: connections, client: client}
}

func (service *ResourceDNS) Options(ctx context.Context) ([]EnvironmentDNSOption, error) {
	return models.DNSZone.EnvironmentOptions(ctx, service.db.Executor())
}

func (service *ResourceDNS) ValidateSelection(
	ctx context.Context,
	hostname, mode string,
	zoneID *uuid.UUID,
) error {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" || mode == DNSModeManual {
		return nil
	}
	if mode != DNSModeCloudflare || zoneID == nil || *zoneID == uuid.Nil {
		return domainError(
			"dnsZoneId",
			"required",
			"select a Cloudflare DNS zone or use manual DNS",
		)
	}
	zone, err := models.DNSZone.Find(ctx, service.db.Executor(), *zoneID)
	if err != nil || zone.ArchivedAt.Valid || zone.Status != "active" {
		return domainError(
			"dnsZoneId",
			"unavailable",
			"selected Cloudflare DNS zone is unavailable",
		)
	}
	if !hostnameBelongsToZone(hostname, zone.Name) {
		return domainError(
			"hostname",
			"zone",
			fmt.Sprintf("hostname must be %s or a subdomain of %s", zone.Name, zone.Name),
		)
	}
	return nil
}

func (service *ResourceDNS) Configure(
	ctx context.Context,
	endpoint models.ResourceEndpointEntity,
	input ResourceCaddyPublicationInput,
) error {
	mode := strings.ToLower(strings.TrimSpace(input.DNSMode))
	if mode == "" {
		mode = DNSModeManual
	}
	if mode == DNSModeManual {
		return service.Remove(ctx, endpoint.ID)
	}
	if err := service.ValidateSelection(ctx, endpoint.Address, mode, input.DNSZoneID); err != nil {
		return err
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	binding, err := models.ResourceDNSBinding.ActiveForEndpoint(ctx, tx, endpoint.ID)
	if errors.Is(err, sql.ErrNoRows) {
		binding, err = models.ResourceDNSBinding.Create(ctx, tx, endpoint.ID, *input.DNSZoneID)
	} else if err == nil && binding.DNSZoneID != *input.DNSZoneID {
		binding, err = models.ResourceDNSBinding.Reconfigure(ctx, tx, binding.ID, *input.DNSZoneID)
	}
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return service.Reconcile(ctx, endpoint.ID)
}

func (service *ResourceDNS) Reconcile(ctx context.Context, endpointID uuid.UUID) error {
	endpoint, err := models.ResourceEndpoint.Find(ctx, service.db.Executor(), endpointID)
	if err != nil {
		return err
	}
	binding, err := models.ResourceDNSBinding.ActiveForEndpoint(
		ctx,
		service.db.Executor(),
		endpointID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := models.ResourceDNSBinding.MarkState(
		ctx,
		service.db.Executor(),
		binding.ID,
		models.EnvironmentDNSReconciling,
		"",
		now,
	); err != nil {
		return err
	}
	if err := service.reconcile(ctx, endpoint, binding); err != nil {
		message := strings.TrimSpace(err.Error())
		if len(message) > 1000 {
			message = message[:1000]
		}
		state := models.EnvironmentDNSFailed
		if errors.Is(err, errResourceDNSConflict) {
			state = models.EnvironmentDNSConflict
		}
		_ = models.ResourceDNSBinding.MarkState(
			ctx,
			service.db.Executor(),
			binding.ID,
			state,
			message,
			time.Now().UTC(),
		)
		return err
	}
	return models.ResourceDNSBinding.MarkState(
		ctx,
		service.db.Executor(),
		binding.ID,
		models.EnvironmentDNSApplied,
		"",
		time.Now().UTC(),
	)
}

var errResourceDNSConflict = errors.New("Resource DNS hostname has unmanaged address records")

func (service *ResourceDNS) reconcile(
	ctx context.Context,
	endpoint models.ResourceEndpointEntity,
	binding models.ResourceDNSBindingEntity,
) error {
	zone, err := models.DNSZone.Find(ctx, service.db.Executor(), binding.DNSZoneID)
	if err != nil || zone.ArchivedAt.Valid || zone.Status != "active" {
		return errors.New("selected Cloudflare DNS zone is unavailable")
	}
	_, token, err := service.connections.connectionToken(
		ctx,
		service.db.Executor(),
		zone.DNSConnectionID,
	)
	if err != nil {
		return err
	}
	desired, err := service.desiredIPv4(ctx, endpoint.ResourceID)
	if err != nil {
		return err
	}
	remote, err := service.client.ListAddressRecords(ctx, token, zone.ExternalID, endpoint.Address)
	if err != nil {
		return err
	}
	tracked, err := models.ResourceDNSRecord.ActiveForBinding(
		ctx,
		service.db.Executor(),
		binding.ID,
	)
	if err != nil {
		return err
	}
	trackedIDs := make(map[string]struct{}, len(tracked))
	for _, record := range tracked {
		if record.DNSZoneID == zone.ID && strings.EqualFold(record.ObservedName, endpoint.Address) {
			trackedIDs[record.ExternalID] = struct{}{}
		}
	}
	marker := resourceDNSOwnershipMarker(binding.ID)
	owned := make([]cloudflareclient.DNSRecord, 0)
	unmanaged := 0
	for _, record := range remote {
		_, wasTracked := trackedIDs[record.ID]
		if wasTracked || record.Comment == marker {
			owned = append(owned, record)
		} else {
			unmanaged++
		}
	}
	if unmanaged > 0 {
		return fmt.Errorf(
			"%w: %d unmanaged Cloudflare address record(s) already use %s",
			errResourceDNSConflict,
			unmanaged,
			endpoint.Address,
		)
	}
	sort.Slice(owned, func(i, j int) bool { return owned[i].ID < owned[j].ID })
	applied := make([]cloudflareclient.DNSRecord, 0, len(desired))
	for index, address := range desired {
		input := cloudflareclient.DNSRecordInput{
			Type:    "A",
			Name:    endpoint.Address,
			Content: address,
			TTL:     1,
			Proxied: true,
			Comment: marker,
		}
		var record cloudflareclient.DNSRecord
		if index < len(owned) && strings.EqualFold(owned[index].Type, "A") {
			record, err = service.client.UpdateARecord(
				ctx,
				token,
				zone.ExternalID,
				owned[index].ID,
				input,
			)
		} else {
			if index < len(owned) {
				err = service.client.DeleteRecord(ctx, token, zone.ExternalID, owned[index].ID)
			}
			if err == nil {
				record, err = service.client.CreateARecord(ctx, token, zone.ExternalID, input)
			}
		}
		if err != nil {
			return err
		}
		applied = append(applied, record)
	}
	for index := len(desired); index < len(owned); index++ {
		if err := service.client.DeleteRecord(
			ctx,
			token,
			zone.ExternalID,
			owned[index].ID,
		); err != nil {
			return err
		}
	}
	if err := service.removeTracked(ctx, binding.ID, func(record dnsTrackedRemoval) bool {
		return record.ZoneID != zone.ID || !strings.EqualFold(record.ObservedName, endpoint.Address)
	}); err != nil {
		return err
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	externalIDs := make([]string, 0, len(applied))
	for _, record := range applied {
		externalIDs = append(externalIDs, record.ID)
		if _, err := models.ResourceDNSRecord.Upsert(ctx, tx, models.UpsertResourceDNSRecordData{
			ExternalID: record.ID, Content: record.Content, ObservedName: record.Name,
			ResourceDNSBindingID: binding.ID, DNSZoneID: zone.ID,
		}); err != nil {
			return err
		}
	}
	if err := models.ResourceDNSRecord.ArchiveMissing(
		ctx,
		tx,
		binding.ID,
		externalIDs,
		time.Now().UTC(),
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *ResourceDNS) desiredIPv4(
	ctx context.Context,
	resourceID uuid.UUID,
) ([]string, error) {
	servers, err := models.ResourceInstallation.ActiveDNSServerAddresses(
		ctx,
		service.db.Executor(),
		resourceID,
	)
	if err != nil {
		return nil, err
	}
	values := make([]string, 0, len(servers))
	seen := make(map[string]struct{}, len(servers))
	for _, server := range servers {
		value, err := (&EnvironmentDNS{db: service.db}).resolvePublicIPv4(
			ctx,
			service.db.Executor(),
			server,
		)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[value]; !ok {
			seen[value] = struct{}{}
			values = append(values, value)
		}
	}
	if len(values) == 0 {
		return nil, errors.Join(
			models.ErrDomainValidation,
			errors.New("Cloudflare-managed DNS requires an active Resource installation Server"),
		)
	}
	sort.Strings(values)
	return values, nil
}

func (service *ResourceDNS) Remove(ctx context.Context, endpointID uuid.UUID) error {
	binding, err := models.ResourceDNSBinding.ActiveForEndpoint(
		ctx,
		service.db.Executor(),
		endpointID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := service.removeTracked(ctx, binding.ID, nil); err != nil {
		_ = models.ResourceDNSBinding.MarkState(
			ctx,
			service.db.Executor(),
			binding.ID,
			models.EnvironmentDNSRemovalFailed,
			err.Error(),
			time.Now().UTC(),
		)
		return err
	}
	now := time.Now().UTC()
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := models.ResourceDNSRecord.ArchiveMissing(ctx, tx, binding.ID, nil, now); err != nil {
		return err
	}
	if err := models.ResourceDNSBinding.Archive(ctx, tx, binding.ID, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *ResourceDNS) removeTracked(
	ctx context.Context,
	bindingID uuid.UUID,
	remove func(dnsTrackedRemoval) bool,
) error {
	records, err := models.ResourceDNSRecord.TrackedRemovals(ctx, service.db.Executor(), bindingID)
	if err != nil {
		return err
	}
	for _, record := range records {
		if remove != nil && !remove(record) {
			continue
		}
		token, err := secretcrypto.DecryptForPurpose(
			record.CredentialPayload,
			service.connections.config.App.SessionEncryptionKey,
			cloudflareTokenEncryptionPurpose,
		)
		if err != nil {
			return errors.New("Cloudflare account-owned API token could not be decrypted")
		}
		if err := service.client.DeleteRecord(
			ctx,
			string(token),
			record.ZoneExternalID,
			record.ExternalID,
		); err != nil {
			return err
		}
	}
	return nil
}

func (service *ResourceDNS) Status(
	ctx context.Context,
	endpointID uuid.UUID,
) (ResourceDNSStatus, error) {
	status := ResourceDNSStatus{Mode: DNSModeManual, State: "manual", Records: []DNSRecordStatus{}}
	row, err := models.ResourceDNSBinding.StatusForEndpoint(ctx, service.db.Executor(), endpointID)
	if errors.Is(err, sql.ErrNoRows) {
		return status, nil
	}
	if err != nil {
		return ResourceDNSStatus{}, err
	}
	status.Mode, status.ZoneID, status.ZoneName = DNSModeCloudflare, row.ZoneID.String(), row.ZoneName
	status.ConnectionName, status.State, status.LastError = row.ConnectionName, row.State, row.LastError.String
	if row.AppliedAt.Valid {
		status.AppliedAt = row.AppliedAt.Time.UTC().Format(time.RFC3339)
	}
	status.Records, err = service.resourceRecordStatus(ctx, row.BindingID)
	return status, err
}

func (service *ResourceDNS) resourceRecordStatus(
	ctx context.Context,
	bindingID uuid.UUID,
) ([]DNSRecordStatus, error) {
	rows := make([]DNSRecordStatus, 0)
	err := service.db.Executor().NewSelect().TableExpr("resource_dns_records").
		ColumnExpr("record_type, observed_name, content").
		Where("resource_dns_binding_id = ?", bindingID).
		Where("archived_at IS NULL").OrderExpr("content").Scan(ctx, &rows)
	return rows, err
}

func resourceDNSOwnershipMarker(bindingID uuid.UUID) string {
	return "deploycrate:resource-dns-binding:" + bindingID.String()
}
