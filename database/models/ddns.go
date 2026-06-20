package models

const (
	DDNSProviderCloudflare = "cloudflare"

	DDNSRecordTypeA    = "A"
	DDNSRecordTypeAAAA = "AAAA"

	DDNSIPSourceClientIPv4 = "client_ipv4"
	DDNSIPSourceClientIPv6 = "client_ipv6"

	DDNSStatusSuccess   = "success"
	DDNSStatusError     = "error"
	DDNSStatusUnchanged = "unchanged"
)

type DDNSProviderSetting struct {
	Provider       string    `json:"provider" gorm:"type:varchar(50);primaryKey"`
	EncryptedToken string    `json:"-" gorm:"type:longtext"`
	CreatedAt      LocalTime `json:"created_at"`
	UpdatedAt      LocalTime `json:"updated_at"`
}

type DDNSRecord struct {
	Id         uint   `json:"id,omitempty" gorm:"primaryKey;autoIncrement"`
	Enabled    bool   `json:"enabled" gorm:"type:boolean;default:true;index"`
	Provider   string `json:"provider" gorm:"type:varchar(50);not null;default:'cloudflare';index"`
	Client     string `json:"client" gorm:"type:varchar(36);not null;index"`
	ClientInfo Client `json:"client_info,omitempty" gorm:"foreignKey:Client;references:UUID;constraint:OnDelete:CASCADE,OnUpdate:CASCADE"`

	ZoneID     string `json:"zone_id" gorm:"type:varchar(100);not null"`
	RecordID   string `json:"record_id" gorm:"type:varchar(100);not null"`
	RecordName string `json:"record_name" gorm:"type:varchar(255);not null;index"`
	RecordType string `json:"record_type" gorm:"type:varchar(10);not null;default:'A'"`
	IPSource   string `json:"ip_source" gorm:"type:varchar(30);not null;default:'client_ipv4'"`
	TTL        int    `json:"ttl" gorm:"type:int;not null;default:1"`
	Proxied    bool   `json:"proxied" gorm:"type:boolean;default:false"`

	CurrentIP  string    `json:"current_ip,omitempty" gorm:"type:varchar(100)"`
	LastIP     string    `json:"last_ip,omitempty" gorm:"type:varchar(100)"`
	LastStatus string    `json:"last_status,omitempty" gorm:"type:varchar(20)"`
	LastError  string    `json:"last_error,omitempty" gorm:"type:text"`
	LastSyncAt LocalTime `json:"last_sync_at"`
	CreatedAt  LocalTime `json:"created_at"`
	UpdatedAt  LocalTime `json:"updated_at"`
}
