package ddns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/utils/cloudflareddns"
	"github.com/komari-monitor/komari/utils/secureconfig"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const cloudflareDDNSTokenEnv = "KOMARI_CLOUDFLARE_DDNS_TOKEN"

var syncMu sync.Mutex

type ProviderStatus struct {
	Provider           string           `json:"provider"`
	TokenConfigured    bool             `json:"token_configured"`
	EnvTokenConfigured bool             `json:"env_token_configured"`
	UpdatedAt          models.LocalTime `json:"updated_at,omitempty"`
}

type SyncResult struct {
	Id         uint   `json:"id"`
	Client     string `json:"client"`
	RecordName string `json:"record_name"`
	RecordType string `json:"record_type"`
	IP         string `json:"ip,omitempty"`
	Status     string `json:"status"`
	Updated    bool   `json:"updated"`
	Message    string `json:"message,omitempty"`
}

func SaveProviderToken(provider string, token string) error {
	provider, err := normalizeProvider(provider)
	if err != nil {
		return err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("cloudflare token is required")
	}
	encrypted, err := secureconfig.EncryptString(token)
	if err != nil {
		return err
	}
	now := models.FromTime(time.Now())
	setting := models.DDNSProviderSetting{
		Provider:       provider,
		EncryptedToken: encrypted,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	return dbcore.GetDBInstance().Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "provider"}},
		DoUpdates: clause.AssignmentColumns([]string{"encrypted_token", "updated_at"}),
	}).Create(&setting).Error
}

func RemoveProviderToken(provider string) error {
	provider, err := normalizeProvider(provider)
	if err != nil {
		return err
	}
	return dbcore.GetDBInstance().Where("provider = ?", provider).Delete(&models.DDNSProviderSetting{}).Error
}

func GetProviderStatus(provider string) (ProviderStatus, error) {
	provider, err := normalizeProvider(provider)
	if err != nil {
		return ProviderStatus{}, err
	}
	status := ProviderStatus{
		Provider:           provider,
		EnvTokenConfigured: strings.TrimSpace(os.Getenv(cloudflareDDNSTokenEnv)) != "",
	}
	if status.EnvTokenConfigured {
		status.TokenConfigured = true
	}
	var setting models.DDNSProviderSetting
	err = dbcore.GetDBInstance().Where("provider = ?", provider).First(&setting).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return status, nil
		}
		return ProviderStatus{}, err
	}
	status.UpdatedAt = setting.UpdatedAt
	if setting.EncryptedToken != "" {
		status.TokenConfigured = true
	}
	return status, nil
}

func LoadProviderToken(provider string) (string, error) {
	provider, err := normalizeProvider(provider)
	if err != nil {
		return "", err
	}
	if token := strings.TrimSpace(os.Getenv(cloudflareDDNSTokenEnv)); token != "" {
		return token, nil
	}
	var setting models.DDNSProviderSetting
	err = dbcore.GetDBInstance().Where("provider = ?", provider).First(&setting).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	if strings.TrimSpace(setting.EncryptedToken) == "" {
		return "", nil
	}
	token, err := secureconfig.DecryptString(setting.EncryptedToken)
	if err == nil {
		return token, nil
	}
	return setting.EncryptedToken, nil
}

func ListRecords() ([]models.DDNSRecord, error) {
	var records []models.DDNSRecord
	err := dbcore.GetDBInstance().
		Preload("ClientInfo").
		Order("enabled DESC").
		Order("id ASC").
		Find(&records).Error
	return records, err
}

func GetRecord(id uint) (models.DDNSRecord, error) {
	var record models.DDNSRecord
	err := dbcore.GetDBInstance().Preload("ClientInfo").Where("id = ?", id).First(&record).Error
	return record, err
}

func SaveRecord(record *models.DDNSRecord) error {
	if record == nil {
		return fmt.Errorf("ddns record is required")
	}
	if err := normalizeRecord(record); err != nil {
		return err
	}
	if err := ensureClientExists(record.Client); err != nil {
		return err
	}

	db := dbcore.GetDBInstance()
	now := models.FromTime(time.Now())
	if record.Id == 0 {
		record.CreatedAt = now
		record.UpdatedAt = now
		return db.Create(record).Error
	}
	updates := map[string]any{
		"enabled":     record.Enabled,
		"provider":    record.Provider,
		"client":      record.Client,
		"zone_id":     record.ZoneID,
		"record_id":   record.RecordID,
		"record_name": record.RecordName,
		"record_type": record.RecordType,
		"ip_source":   record.IPSource,
		"ttl":         record.TTL,
		"proxied":     record.Proxied,
		"updated_at":  now,
	}
	result := db.Model(&models.DDNSRecord{}).Where("id = ?", record.Id).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func DeleteRecords(ids []uint) error {
	if len(ids) == 0 {
		return fmt.Errorf("id is required")
	}
	result := dbcore.GetDBInstance().Where("id IN ?", ids).Delete(&models.DDNSRecord{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func SetRecordsEnabled(ids []uint, enabled bool) error {
	if len(ids) == 0 {
		return fmt.Errorf("id is required")
	}
	result := dbcore.GetDBInstance().Model(&models.DDNSRecord{}).Where("id IN ?", ids).Updates(map[string]any{
		"enabled":    enabled,
		"updated_at": models.FromTime(time.Now()),
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func SyncAll(ctx context.Context) ([]SyncResult, error) {
	if !syncMu.TryLock() {
		return nil, fmt.Errorf("ddns sync is already running")
	}
	defer syncMu.Unlock()

	var records []models.DDNSRecord
	err := dbcore.GetDBInstance().
		Preload("ClientInfo").
		Where("enabled = ?", true).
		Order("id ASC").
		Find(&records).Error
	if err != nil {
		return nil, err
	}

	results := make([]SyncResult, 0, len(records))
	for _, record := range records {
		results = append(results, syncRecord(ctx, record, false))
	}
	return results, nil
}

func SyncOne(ctx context.Context, id uint, force bool) (SyncResult, error) {
	if !syncMu.TryLock() {
		return SyncResult{}, fmt.Errorf("ddns sync is already running")
	}
	defer syncMu.Unlock()

	record, err := GetRecord(id)
	if err != nil {
		return SyncResult{}, err
	}
	return syncRecord(ctx, record, force), nil
}

func syncRecord(ctx context.Context, record models.DDNSRecord, force bool) SyncResult {
	result := SyncResult{
		Id:         record.Id,
		Client:     record.Client,
		RecordName: record.RecordName,
		RecordType: record.RecordType,
	}
	if !record.Enabled {
		result.Status = models.DDNSStatusUnchanged
		result.Message = "record is disabled"
		return result
	}
	if err := normalizeRecord(&record); err != nil {
		return result.withSavedError(record, err)
	}
	token, err := LoadProviderToken(record.Provider)
	if err != nil {
		return result.withSavedError(record, err)
	}
	if strings.TrimSpace(token) == "" {
		return result.withSavedError(record, fmt.Errorf("cloudflare token is not configured"))
	}
	ip, err := resolveRecordIP(record)
	if err != nil {
		return result.withSavedError(record, err)
	}
	result.IP = ip

	if !force && strings.TrimSpace(record.CurrentIP) == ip && strings.TrimSpace(record.LastError) == "" {
		result.Status = models.DDNSStatusUnchanged
		result.Message = "ip unchanged"
		saveSyncStatus(record.Id, record.CurrentIP, record.LastIP, models.DDNSStatusUnchanged, "")
		return result
	}

	err = cloudflareddns.UpdateDNSRecord(ctx, cloudflareddns.UpdateRequest{
		Token:      token,
		ZoneID:     record.ZoneID,
		RecordID:   record.RecordID,
		RecordName: record.RecordName,
		RecordType: record.RecordType,
		Content:    ip,
		TTL:        record.TTL,
		Proxied:    record.Proxied,
	})
	if err != nil {
		return result.withSavedError(record, sanitizeError(err, token))
	}

	result.Status = models.DDNSStatusSuccess
	result.Updated = true
	result.Message = "dns record updated"
	saveSyncStatus(record.Id, ip, record.CurrentIP, models.DDNSStatusSuccess, "")
	return result
}

func (r SyncResult) withSavedError(record models.DDNSRecord, err error) SyncResult {
	r.Status = models.DDNSStatusError
	r.Message = err.Error()
	saveSyncStatus(record.Id, record.CurrentIP, record.LastIP, models.DDNSStatusError, r.Message)
	return r
}

func saveSyncStatus(id uint, currentIP string, lastIP string, status string, lastError string) {
	if id == 0 {
		return
	}
	updates := map[string]any{
		"current_ip":   currentIP,
		"last_ip":      lastIP,
		"last_status":  status,
		"last_error":   lastError,
		"last_sync_at": models.FromTime(time.Now()),
		"updated_at":   models.FromTime(time.Now()),
	}
	_ = dbcore.GetDBInstance().Model(&models.DDNSRecord{}).Where("id = ?", id).Updates(updates).Error
}

func resolveRecordIP(record models.DDNSRecord) (string, error) {
	client := record.ClientInfo
	if client.UUID == "" {
		if err := dbcore.GetDBInstance().Where("uuid = ?", record.Client).First(&client).Error; err != nil {
			return "", err
		}
	}

	var ipText string
	switch record.IPSource {
	case models.DDNSIPSourceClientIPv4:
		ipText = client.IPv4
	case models.DDNSIPSourceClientIPv6:
		ipText = client.IPv6
	default:
		return "", fmt.Errorf("unsupported ip_source: %s", record.IPSource)
	}

	ipText = strings.TrimSpace(ipText)
	if ipText == "" {
		return "", fmt.Errorf("client %s has no %s address yet", record.Client, record.IPSource)
	}
	ip := net.ParseIP(ipText)
	if ip == nil {
		return "", fmt.Errorf("client %s reported invalid ip: %s", record.Client, ipText)
	}
	if record.RecordType == models.DDNSRecordTypeA && ip.To4() == nil {
		return "", fmt.Errorf("A record requires an IPv4 address")
	}
	if record.RecordType == models.DDNSRecordTypeAAAA && ip.To4() != nil {
		return "", fmt.Errorf("AAAA record requires an IPv6 address")
	}
	return ipText, nil
}

func ensureClientExists(uuid string) error {
	var count int64
	if err := dbcore.GetDBInstance().Model(&models.Client{}).Where("uuid = ?", uuid).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("client not found: %s", uuid)
	}
	return nil
}

func normalizeRecord(record *models.DDNSRecord) error {
	provider, err := normalizeProvider(record.Provider)
	if err != nil {
		return err
	}
	record.Provider = provider
	record.Client = strings.TrimSpace(record.Client)
	record.ZoneID = strings.TrimSpace(record.ZoneID)
	record.RecordID = strings.TrimSpace(record.RecordID)
	record.RecordName = strings.TrimSpace(record.RecordName)
	record.RecordType = strings.ToUpper(strings.TrimSpace(record.RecordType))
	record.IPSource = strings.TrimSpace(record.IPSource)

	if record.Client == "" {
		return fmt.Errorf("client is required")
	}
	if record.ZoneID == "" || record.RecordID == "" || record.RecordName == "" {
		return fmt.Errorf("zone_id, record_id and record_name are required")
	}
	switch record.RecordType {
	case "":
		record.RecordType = models.DDNSRecordTypeA
	case models.DDNSRecordTypeA, models.DDNSRecordTypeAAAA:
	default:
		return fmt.Errorf("record_type must be A or AAAA")
	}
	if record.IPSource == "" {
		if record.RecordType == models.DDNSRecordTypeAAAA {
			record.IPSource = models.DDNSIPSourceClientIPv6
		} else {
			record.IPSource = models.DDNSIPSourceClientIPv4
		}
	}
	if record.RecordType == models.DDNSRecordTypeA && record.IPSource != models.DDNSIPSourceClientIPv4 {
		return fmt.Errorf("A record must use client_ipv4")
	}
	if record.RecordType == models.DDNSRecordTypeAAAA && record.IPSource != models.DDNSIPSourceClientIPv6 {
		return fmt.Errorf("AAAA record must use client_ipv6")
	}
	if record.TTL == 0 {
		record.TTL = 1
	}
	if record.TTL != 1 && (record.TTL < 60 || record.TTL > 86400) {
		return fmt.Errorf("ttl must be 1 for automatic or between 60 and 86400 seconds")
	}
	return nil
}

func normalizeProvider(provider string) (string, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		provider = models.DDNSProviderCloudflare
	}
	if provider != models.DDNSProviderCloudflare {
		return "", fmt.Errorf("unsupported ddns provider: %s", provider)
	}
	return provider, nil
}

func sanitizeError(err error, token string) error {
	message := err.Error()
	token = strings.TrimSpace(token)
	if token != "" {
		message = strings.ReplaceAll(message, token, "[redacted]")
	}
	return errors.New(message)
}
