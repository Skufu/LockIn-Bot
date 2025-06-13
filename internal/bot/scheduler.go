package bot

import (
	"context"
	"log"
	"time"

	"github.com/robfig/cron/v3"
)

// Scheduler handles periodic tasks for the bot
type Scheduler struct {
	bot  *Bot
	cron *cron.Cron
}

// NewScheduler creates a new scheduler for the bot
func NewScheduler(bot *Bot) *Scheduler {
	cronInstance := cron.New(cron.WithSeconds())
	return &Scheduler{
		bot:  bot,
		cron: cronInstance,
	}
}

// Start starts the scheduler
func (s *Scheduler) Start() {
	// Reset daily study time at midnight
	_, err := s.cron.AddFunc("0 0 0 * * *", func() {
		log.Println("Resetting daily study time")
		ctx := context.Background()
		err := s.bot.db.ResetDailyStudyTime(ctx)
		if err != nil {
			log.Printf("Error resetting daily study time: %v", err)
		}
	})
	if err != nil {
		log.Printf("Error adding daily reset job: %v", err)
	}

	// Reset weekly study time at midnight on Sunday
	_, err = s.cron.AddFunc("0 0 0 * * 0", func() {
		log.Println("Resetting weekly study time")
		ctx := context.Background()
		err := s.bot.db.ResetWeeklyStudyTime(ctx)
		if err != nil {
			log.Printf("Error resetting weekly study time: %v", err)
		}
	})
	if err != nil {
		log.Printf("Error adding weekly reset job: %v", err)
	}

	// Reset monthly study time at midnight on the 1st of each month
	_, err = s.cron.AddFunc("0 0 0 1 * *", func() {
		log.Println("Resetting monthly study time")
		ctx := context.Background()
		err := s.bot.db.ResetMonthlyStudyTime(ctx)
		if err != nil {
			log.Printf("Error resetting monthly study time: %v", err)
		}
	})
	if err != nil {
		log.Printf("Error adding monthly reset job: %v", err)
	}

	// Job to delete old study sessions (older than 1 week)
	// Runs daily at 3:05 AM server time
	_, err = s.cron.AddFunc("0 5 3 * * *", func() {
		log.Println("Running job to delete old study sessions (older than 1 week)...")
		ctx := context.Background()
		// Calculate the cutoff date (1 week ago)
		cutoffDate := time.Now().AddDate(0, 0, -7)

		err := s.bot.db.DeleteOldStudySessions(ctx, cutoffDate)
		if err != nil {
			log.Printf("Error deleting old study sessions: %v", err)
		} else {
			log.Println("Successfully completed job to delete old study sessions.")
		}
	})
	if err != nil {
		log.Printf("Error adding job to delete old study sessions: %v", err)
	}

	s.cron.Start()
	log.Println("Scheduler started")
}

// Stop stops the scheduler
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
	log.Println("Scheduler stopped")
}
