package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type MemberApplication struct {
	ID string `gorm:"type:uuid;primaryKey"`

	LineUserID      *string `gorm:"size:128;index:idx_member_applications_lookup_line_user_id"`
	LineDisplayName string  `gorm:"size:255"`
	LinePictureURL  string  `gorm:"size:1024"`

	FirstName   string `gorm:"size:120;not null"`
	LastName    string `gorm:"size:120;not null"`
	Nickname    string `gorm:"size:120;not null"`
	Phone       string `gorm:"size:20;not null;index:idx_member_applications_lookup_phone"`
	CitizenID   string `gorm:"size:13;not null;index:idx_member_applications_lookup_citizen_id"`
	ShopPageURL string `gorm:"size:1024;not null"`

	StorefrontImagePath string `gorm:"size:1024;not null"`
	StorefrontImageURL  string `gorm:"size:1024;not null"`
	Status              string `gorm:"size:32;not null;default:pending;index:idx_member_applications_status"`

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (m *MemberApplication) BeforeCreate(_ *gorm.DB) error {
	if m.ID == "" {
		m.ID = uuid.NewString()
	}

	return nil
}

type Member struct {
	ID string `gorm:"type:uuid;primaryKey"`

	ApplicationID string `gorm:"type:uuid;not null;uniqueIndex"`

	LineUserID      *string `gorm:"size:128;uniqueIndex"`
	LineDisplayName string  `gorm:"size:255"`
	LinePictureURL  string  `gorm:"size:1024"`

	FirstName   string `gorm:"size:120;not null"`
	LastName    string `gorm:"size:120;not null"`
	Nickname    string `gorm:"size:120;not null"`
	Phone       string `gorm:"size:20;not null;uniqueIndex"`
	CitizenID   string `gorm:"size:13;not null;uniqueIndex"`
	ShopPageURL string `gorm:"size:1024;not null"`

	StorefrontImagePath string `gorm:"size:1024;not null"`
	StorefrontImageURL  string `gorm:"size:1024;not null"`

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (m *Member) BeforeCreate(_ *gorm.DB) error {
	if m.ID == "" {
		m.ID = uuid.NewString()
	}

	return nil
}

type PushSubscription struct {
	ID string `gorm:"type:uuid;primaryKey"`

	Endpoint string `gorm:"size:2048;not null;uniqueIndex"`
	P256DH   string `gorm:"size:512;not null"`
	Auth     string `gorm:"size:512;not null"`

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (p *PushSubscription) BeforeCreate(_ *gorm.DB) error {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}

	return nil
}

type RichMenuSyncState struct {
	Scope string `gorm:"size:64;primaryKey"`

	RichMenuID string `gorm:"size:128;not null"`
	Status     string `gorm:"size:32;not null"`
	Total      int
	Success    int
	Failed     int
	LastError  string `gorm:"size:2048"`
	SyncedAt   *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

type LegacyRegisteredMember struct {
	ID string `gorm:"type:uuid;primaryKey"`

	LineUserID      *string
	LineDisplayName string
	LinePictureURL  string

	FirstName   string
	LastName    string
	Nickname    string
	Phone       string
	CitizenID   string
	ShopPageURL string

	StorefrontImagePath string
	StorefrontImageURL  string
	Status              string

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt
}

func (LegacyRegisteredMember) TableName() string {
	return "registered_members"
}
