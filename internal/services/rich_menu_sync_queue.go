package services

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/fulltank-garage/fulltankgarage-api/internal/cache"
	"github.com/fulltank-garage/fulltankgarage-api/internal/realtime"
)

const richMenuSyncQueueKey = "queue:rich-menu-sync"

type RichMenuSyncJob struct {
	LineUserID   string `json:"lineUserId"`
	SerialNumber string `json:"serialNumber"`
	Source       string `json:"source"`
	Attempts     int    `json:"attempts"`
}

type RichMenuSyncQueue struct {
	cache    *cache.Store
	richMenu *RichMenuService
	events   *realtime.Hub
}

func NewRichMenuSyncQueue(cacheStore *cache.Store, richMenu *RichMenuService, events *realtime.Hub) *RichMenuSyncQueue {
	return &RichMenuSyncQueue{
		cache:    cacheStore,
		richMenu: richMenu,
		events:   events,
	}
}

func (q *RichMenuSyncQueue) EnqueueMemberLink(ctx context.Context, lineUserID string, serialNumber string, source string) {
	if q == nil || q.cache == nil || strings.TrimSpace(lineUserID) == "" {
		return
	}

	job := RichMenuSyncJob{
		LineUserID:   strings.TrimSpace(lineUserID),
		SerialNumber: strings.TrimSpace(serialNumber),
		Source:       strings.TrimSpace(source),
	}
	if job.Source == "" {
		job.Source = "rich_menu_retry"
	}

	if err := q.cache.EnqueueJSON(ctx, richMenuSyncQueueKey, job); err != nil {
		log.Printf("enqueue rich menu sync job: %v", err)
	}
}

func (q *RichMenuSyncQueue) Start(ctx context.Context) {
	if q == nil || q.cache == nil || q.cache.Client() == nil || q.richMenu == nil {
		return
	}

	go q.run(ctx)
}

func (q *RichMenuSyncQueue) run(ctx context.Context) {
	client := q.cache.Client()
	for {
		if ctx.Err() != nil {
			return
		}

		result, err := client.BRPop(ctx, 5*time.Second, richMenuSyncQueueKey).Result()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		if len(result) < 2 {
			continue
		}

		var job RichMenuSyncJob
		if err := json.Unmarshal([]byte(result[1]), &job); err != nil {
			log.Printf("decode rich menu sync job: %v", err)
			continue
		}

		q.process(ctx, job)
	}
}

func (q *RichMenuSyncQueue) process(ctx context.Context, job RichMenuSyncJob) {
	lineUserID := strings.TrimSpace(job.LineUserID)
	if lineUserID == "" {
		return
	}

	err := q.richMenu.LinkMemberRichMenu(ctx, lineUserID)
	if err != nil {
		log.Printf("retry rich menu sync failed lineUserID=%s serial=%s attempts=%d: %v", lineUserID, job.SerialNumber, job.Attempts+1, err)
		q.publish(job, false, "", err.Error())
		if job.Attempts < 5 {
			job.Attempts++
			go func() {
				timer := time.NewTimer(time.Duration(job.Attempts) * 10 * time.Second)
				defer timer.Stop()
				select {
				case <-ctx.Done():
				case <-timer.C:
					_ = q.cache.EnqueueJSON(context.Background(), richMenuSyncQueueKey, job)
				}
			}()
		}
		return
	}

	linkedRichMenuID := q.richMenu.MemberRichMenuID()
	if currentRichMenuID, err := q.richMenu.GetUserRichMenuID(ctx, lineUserID); err == nil && currentRichMenuID != "" {
		linkedRichMenuID = currentRichMenuID
	}
	q.publish(job, true, linkedRichMenuID, "")
}

func (q *RichMenuSyncQueue) publish(job RichMenuSyncJob, success bool, linkedRichMenuID string, message string) {
	if q.events == nil {
		return
	}

	q.events.Publish(realtime.Event{
		Type: "rich_menu.sync",
		Data: map[string]any{
			"lineUserId":       job.LineUserID,
			"serialNumber":     job.SerialNumber,
			"success":          success,
			"linkedRichMenuId": linkedRichMenuID,
			"targetRichMenuId": q.richMenu.MemberRichMenuID(),
			"source":           job.Source,
			"message":          message,
		},
	})
}
