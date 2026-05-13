package repositories

import (
	"github.com/fulltank-garage/fulltankgarage-api/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PushSubscriptionRepository struct {
	db *gorm.DB
}

func NewPushSubscriptionRepository(db *gorm.DB) *PushSubscriptionRepository {
	return &PushSubscriptionRepository{db: db}
}

func (r *PushSubscriptionRepository) Upsert(subscription *models.PushSubscription) error {
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "endpoint"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"p256_dh",
			"auth",
			"updated_at",
			"deleted_at",
		}),
	}).Create(subscription).Error
}

func (r *PushSubscriptionRepository) List() ([]models.PushSubscription, error) {
	var subscriptions []models.PushSubscription
	err := r.db.Order("created_at DESC").Find(&subscriptions).Error
	return subscriptions, err
}

func (r *PushSubscriptionRepository) FindByEndpoint(endpoint string) (*models.PushSubscription, error) {
	var subscription models.PushSubscription
	if err := r.db.Where("endpoint = ?", endpoint).First(&subscription).Error; err != nil {
		return nil, err
	}

	return &subscription, nil
}

func (r *PushSubscriptionRepository) DeleteByEndpoint(endpoint string) error {
	return r.db.Unscoped().Where("endpoint = ?", endpoint).Delete(&models.PushSubscription{}).Error
}
