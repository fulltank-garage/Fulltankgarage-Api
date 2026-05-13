package repositories

import (
	"errors"

	"github.com/fulltank-garage/fulltankgarage-api/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type MemberRepository struct {
	db *gorm.DB
}

func NewMemberRepository(db *gorm.DB) *MemberRepository {
	return &MemberRepository{db: db}
}

func (r *MemberRepository) CreateApplication(application *models.MemberApplication) error {
	return r.db.Create(application).Error
}

func (r *MemberRepository) ListApplications() ([]models.MemberApplication, error) {
	var applications []models.MemberApplication
	err := r.db.Where("status = ?", "pending").Order("created_at DESC").Find(&applications).Error
	return applications, err
}

func (r *MemberRepository) FindApplicationConflict(lineUserID, citizenID, phone string) (*models.MemberApplication, error) {
	var application models.MemberApplication
	query := r.db.Where("status = ? AND (citizen_id = ? OR phone = ?)", "pending", citizenID, phone)
	if lineUserID != "" {
		query = r.db.Where("status = ? AND (line_user_id = ? OR citizen_id = ? OR phone = ?)", "pending", lineUserID, citizenID, phone)
	}

	if err := query.First(&application).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}

		return nil, err
	}

	return &application, nil
}

func (r *MemberRepository) FindApplicationByID(id string) (*models.MemberApplication, error) {
	var application models.MemberApplication
	if err := r.db.Where("id = ?", id).First(&application).Error; err != nil {
		return nil, err
	}

	return &application, nil
}

func (r *MemberRepository) FindApplicationByLineUserID(lineUserID string) (*models.MemberApplication, error) {
	var application models.MemberApplication
	if err := r.db.Where("line_user_id = ?", lineUserID).Order("created_at DESC").First(&application).Error; err != nil {
		return nil, err
	}

	return &application, nil
}

func (r *MemberRepository) SaveApplication(application *models.MemberApplication) error {
	return r.db.Save(application).Error
}

func (r *MemberRepository) ApproveApplication(application *models.MemberApplication) (*models.Member, error) {
	var member models.Member
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(application).Error; err != nil {
			return err
		}

		nextMember, err := upsertMemberFromApplication(tx, application)
		if err != nil {
			return err
		}

		member = *nextMember
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &member, nil
}

func (r *MemberRepository) DeleteApplication(application *models.MemberApplication) error {
	return r.db.Unscoped().Delete(application).Error
}

func (r *MemberRepository) FindMemberConflict(lineUserID, citizenID, phone string) (*models.Member, error) {
	var member models.Member
	query := r.db.Where("citizen_id = ? OR phone = ?", citizenID, phone)
	if lineUserID != "" {
		query = r.db.Where("line_user_id = ? OR citizen_id = ? OR phone = ?", lineUserID, citizenID, phone)
	}

	if err := query.First(&member).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}

		return nil, err
	}

	return &member, nil
}

func (r *MemberRepository) ListMembers() ([]models.Member, error) {
	var members []models.Member
	err := r.db.Order("created_at DESC").Find(&members).Error
	return members, err
}

func (r *MemberRepository) ListMembersWithLineUserID() ([]models.Member, error) {
	var members []models.Member
	err := r.db.Where("line_user_id IS NOT NULL AND line_user_id <> ''").Order("created_at ASC").Find(&members).Error
	return members, err
}

func (r *MemberRepository) FindMemberByLineUserID(lineUserID string) (*models.Member, error) {
	var member models.Member
	if err := r.db.Where("line_user_id = ?", lineUserID).First(&member).Error; err != nil {
		return nil, err
	}

	return &member, nil
}

func (r *MemberRepository) FindMemberByID(id string) (*models.Member, error) {
	var member models.Member
	if err := r.db.Where("id = ?", id).First(&member).Error; err != nil {
		return nil, err
	}

	return &member, nil
}

func (r *MemberRepository) FindLatestMember() (*models.Member, error) {
	var member models.Member
	if err := r.db.Order("created_at DESC").First(&member).Error; err != nil {
		return nil, err
	}

	return &member, nil
}

func upsertMemberFromApplication(db *gorm.DB, application *models.MemberApplication) (*models.Member, error) {
	member := memberFromApplication(application)
	err := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "application_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"line_user_id",
			"line_display_name",
			"line_picture_url",
			"first_name",
			"last_name",
			"nickname",
			"phone",
			"citizen_id",
			"shop_page_url",
			"storefront_image_path",
			"storefront_image_url",
			"updated_at",
		}),
	}).Create(&member).Error
	if err != nil {
		return nil, err
	}

	var savedMember models.Member
	if err := db.Where("id = ?", member.ID).First(&savedMember).Error; err != nil {
		return nil, err
	}

	return &savedMember, nil
}

func (r *MemberRepository) DeleteMemberWithApplication(member *models.Member) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Delete(member).Error; err != nil {
			return err
		}

		if member.ApplicationID == "" {
			return nil
		}

		return tx.Unscoped().Where("id = ?", member.ApplicationID).Delete(&models.MemberApplication{}).Error
	})
}

func (r *MemberRepository) FindRichMenuSyncState(scope string) (*models.RichMenuSyncState, error) {
	var state models.RichMenuSyncState
	if err := r.db.Where("scope = ?", scope).First(&state).Error; err != nil {
		return nil, err
	}

	return &state, nil
}

func (r *MemberRepository) SaveRichMenuSyncState(state *models.RichMenuSyncState) error {
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "scope"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"rich_menu_id",
			"status",
			"total",
			"success",
			"failed",
			"last_error",
			"synced_at",
			"updated_at",
		}),
	}).Create(state).Error
}

func memberFromApplication(application *models.MemberApplication) models.Member {
	return models.Member{
		ID:                  application.ID,
		ApplicationID:       application.ID,
		LineUserID:          application.LineUserID,
		LineDisplayName:     application.LineDisplayName,
		LinePictureURL:      application.LinePictureURL,
		FirstName:           application.FirstName,
		LastName:            application.LastName,
		Nickname:            application.Nickname,
		Phone:               application.Phone,
		CitizenID:           application.CitizenID,
		ShopPageURL:         application.ShopPageURL,
		StorefrontImagePath: application.StorefrontImagePath,
		StorefrontImageURL:  application.StorefrontImageURL,
	}
}
