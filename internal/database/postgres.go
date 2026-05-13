package database

import (
	"errors"
	"log"
	"os"
	"time"

	"github.com/fulltank-garage/fulltankgarage-api/internal/config"
	"github.com/fulltank-garage/fulltankgarage-api/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Connect(cfg config.Config) (*gorm.DB, error) {
	dbLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  cfg.AppEnv != "production",
		},
	)

	db, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{
		Logger: dbLogger,
	})
	if err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(&models.MemberApplication{}, &models.Member{}, &models.PushSubscription{}, &models.RichMenuSyncState{}, &models.SerialNumber{}, &models.WarrantyRegistration{}, &models.FilmProduct{}, &models.Promotion{}); err != nil {
		return nil, err
	}

	if err := ensureMemberApplicationIndexes(db); err != nil {
		return nil, err
	}

	if err := migrateRegisteredMembers(db); err != nil {
		return nil, err
	}

	result := db.Unscoped().Where("deleted_at IS NOT NULL").Delete(&models.MemberApplication{})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected > 0 {
		log.Printf("purged soft-deleted member applications: %d", result.RowsAffected)
	}

	result = db.Unscoped().Where("deleted_at IS NOT NULL").Delete(&models.Member{})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected > 0 {
		log.Printf("purged soft-deleted members: %d", result.RowsAffected)
	}

	return db, nil
}

func ensureMemberApplicationIndexes(db *gorm.DB) error {
	statements := []string{
		`DROP INDEX IF EXISTS idx_member_applications_line_user_id`,
		`DROP INDEX IF EXISTS idx_member_applications_phone`,
		`DROP INDEX IF EXISTS idx_member_applications_citizen_id`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_member_applications_pending_line_user_id ON member_applications (line_user_id) WHERE deleted_at IS NULL AND status = 'pending' AND line_user_id IS NOT NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_member_applications_pending_phone ON member_applications (phone) WHERE deleted_at IS NULL AND status = 'pending'`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_member_applications_pending_citizen_id ON member_applications (citizen_id) WHERE deleted_at IS NULL AND status = 'pending'`,
		`CREATE INDEX IF NOT EXISTS idx_member_applications_pending_created_at ON member_applications (created_at DESC) WHERE deleted_at IS NULL AND status = 'pending'`,
	}

	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}

	return nil
}

func migrateRegisteredMembers(db *gorm.DB) error {
	if !db.Migrator().HasTable(&models.LegacyRegisteredMember{}) {
		return nil
	}

	var legacyMembers []models.LegacyRegisteredMember
	if err := db.Unscoped().Where("deleted_at IS NULL").Find(&legacyMembers).Error; err != nil {
		return err
	}

	for _, legacy := range legacyMembers {
		application := legacyToApplication(legacy)
		var existingApplication models.MemberApplication
		if err := db.Where("id = ?", application.ID).First(&existingApplication).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err := db.Create(&application).Error; err != nil {
				return err
			}
		}

		if legacy.Status == "approved" {
			member := legacyToMember(legacy)
			var existingMember models.Member
			if err := db.Where("id = ?", member.ID).First(&existingMember).Error; err != nil {
				if !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
				if err := db.Create(&member).Error; err != nil {
					return err
				}
			}
		}
	}

	if len(legacyMembers) > 0 {
		log.Printf("migrated registered_members rows: %d", len(legacyMembers))
	}

	return nil
}

func legacyToApplication(legacy models.LegacyRegisteredMember) models.MemberApplication {
	status := legacy.Status
	if status == "" {
		status = "pending"
	}

	return models.MemberApplication{
		ID:                  legacy.ID,
		LineUserID:          legacy.LineUserID,
		LineDisplayName:     legacy.LineDisplayName,
		LinePictureURL:      legacy.LinePictureURL,
		FirstName:           legacy.FirstName,
		LastName:            legacy.LastName,
		Nickname:            legacy.Nickname,
		Phone:               legacy.Phone,
		CitizenID:           legacy.CitizenID,
		ShopPageURL:         legacy.ShopPageURL,
		StorefrontImagePath: legacy.StorefrontImagePath,
		StorefrontImageURL:  legacy.StorefrontImageURL,
		Status:              status,
		CreatedAt:           legacy.CreatedAt,
		UpdatedAt:           legacy.UpdatedAt,
	}
}

func legacyToMember(legacy models.LegacyRegisteredMember) models.Member {
	return models.Member{
		ID:                  legacy.ID,
		ApplicationID:       legacy.ID,
		LineUserID:          legacy.LineUserID,
		LineDisplayName:     legacy.LineDisplayName,
		LinePictureURL:      legacy.LinePictureURL,
		FirstName:           legacy.FirstName,
		LastName:            legacy.LastName,
		Nickname:            legacy.Nickname,
		Phone:               legacy.Phone,
		CitizenID:           legacy.CitizenID,
		ShopPageURL:         legacy.ShopPageURL,
		StorefrontImagePath: legacy.StorefrontImagePath,
		StorefrontImageURL:  legacy.StorefrontImageURL,
		CreatedAt:           legacy.CreatedAt,
		UpdatedAt:           legacy.UpdatedAt,
	}
}
