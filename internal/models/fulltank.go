package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type StringList []string

func (items StringList) Value() (driver.Value, error) {
	if items == nil {
		return "[]", nil
	}

	bytes, err := json.Marshal(items)
	return string(bytes), err
}

func (items *StringList) Scan(value interface{}) error {
	if value == nil {
		*items = StringList{}
		return nil
	}

	var bytes []byte
	switch typedValue := value.(type) {
	case []byte:
		bytes = typedValue
	case string:
		bytes = []byte(typedValue)
	default:
		*items = StringList{}
		return nil
	}

	if len(bytes) == 0 {
		*items = StringList{}
		return nil
	}

	return json.Unmarshal(bytes, items)
}

type SerialNumber struct {
	ID           uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	SerialNumber string    `gorm:"size:50;not null;uniqueIndex" json:"serialNumber"`
	Status       string    `gorm:"size:16;not null;default:available;index" json:"status"`
	CreatedAt    time.Time `json:"createdAt"`
}

func (SerialNumber) TableName() string {
	return "serial_numbers"
}

type WarrantyRegistration struct {
	ID            uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	UUID          string     `gorm:"type:uuid;uniqueIndex" json:"uuid"`
	SerialNumber  string     `gorm:"size:50;not null;uniqueIndex" json:"serialNumber"`
	CustomerName  string     `gorm:"size:100" json:"customerName"`
	Phone         string     `gorm:"size:20" json:"phone"`
	CarModel      string     `gorm:"size:100" json:"carModel"`
	LicensePlate  string     `gorm:"size:30" json:"licensePlate"`
	FilmBrand     string     `gorm:"size:100" json:"filmBrand"`
	FilmModel     string     `gorm:"size:100" json:"filmModel"`
	InstallDate   *time.Time `gorm:"type:date" json:"installDate"`
	Branch        string     `gorm:"size:100" json:"branch"`
	InstallerName string     `gorm:"size:100" json:"installerName"`
	ReceiptFile   string     `gorm:"size:255" json:"receiptFile"`
	Remarks       string     `gorm:"type:text" json:"remarks"`

	LineUserID      string `gorm:"size:128;index" json:"lineUserId,omitempty"`
	LineDisplayName string `gorm:"size:255" json:"lineDisplayName,omitempty"`
	LinePictureURL  string `gorm:"size:1024" json:"linePictureUrl,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (WarrantyRegistration) TableName() string {
	return "warranty_registrations"
}

func (w *WarrantyRegistration) BeforeCreate(_ *gorm.DB) error {
	if w.UUID == "" {
		w.UUID = uuid.NewString()
	}

	return nil
}

type FilmProduct struct {
	ID              uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	Slug            string     `gorm:"size:120;not null;uniqueIndex" json:"slug"`
	Name            string     `gorm:"size:160;not null" json:"name"`
	Logo            string     `gorm:"size:32" json:"logo"`
	Summary         string     `gorm:"size:255" json:"summary"`
	Description     string     `gorm:"type:text" json:"description"`
	ImageURL        string     `gorm:"size:1024" json:"imageUrl"`
	PriceTableURL   string     `gorm:"size:1024" json:"priceTableImageUrl"`
	GalleryImages   StringList `gorm:"type:jsonb;default:'[]'" json:"galleryImages"`
	IRR             string     `gorm:"size:40" json:"irr"`
	UVProtection    string     `gorm:"size:40" json:"uvProtection"`
	VLT             string     `gorm:"size:40" json:"vlt"`
	TSER            string     `gorm:"size:40" json:"tser"`
	VLR             string     `gorm:"size:40" json:"vlr"`
	FilmType        string     `gorm:"size:80" json:"filmType"`
	VehicleType     string     `gorm:"size:120" json:"vehicleType"`
	InstallPosition string     `gorm:"size:160" json:"installPosition"`
	Highlights      StringList `gorm:"type:jsonb;default:'[]'" json:"highlights"`
	IsActive        bool       `gorm:"not null;default:true" json:"isActive"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

func (FilmProduct) TableName() string {
	return "film_products"
}

type Promotion struct {
	ID          uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	Title       string     `gorm:"size:180;not null" json:"title"`
	Description string     `gorm:"type:text" json:"description"`
	Detail      string     `gorm:"type:text" json:"detail"`
	ImageURL    string     `gorm:"size:1024" json:"imageUrl"`
	IsActive    bool       `gorm:"not null;default:true" json:"isActive"`
	StartsAt    *time.Time `gorm:"type:date" json:"startsAt"`
	EndsAt      *time.Time `gorm:"type:date" json:"endsAt"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

func (Promotion) TableName() string {
	return "promotions"
}
