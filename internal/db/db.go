package db

import (
	"log"
	"os"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Chat struct {
	ID           string `gorm:"primaryKey"`
	Name         string
	IsGroup      bool
	Participants string // comma-separated peer IDs for groups, empty for ALL or direct
}

type Message struct {
	ID         uint   `gorm:"primaryKey;autoIncrement"`
	ChatID     string `gorm:"index"`
	SenderID   string
	SenderName string
	Content    string
	Timestamp  int64
	IsRead     bool
}

var DB *gorm.DB

func InitDB(nodeID string) {
	// DB file per node
	filename := "chat_" + nodeID + ".db"
	
	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			LogLevel: logger.Silent, // don't spam terminal
		},
	)

	var err error
	DB, err = gorm.Open(sqlite.Open(filename), &gorm.Config{
		Logger: newLogger,
	})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	DB.AutoMigrate(&Chat{}, &Message{})

	// Ensure "ALL" chat exists
	var allChat Chat
	if DB.First(&allChat, "id = ?", "ALL").Error != nil {
		DB.Create(&Chat{ID: "ALL", Name: "Global Chat", IsGroup: true, Participants: ""})
	}
}
