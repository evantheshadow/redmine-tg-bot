package main

import (
	"log"
	// "fmt"
	// "reflect"
	"sync"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
)

type User struct {
	gorm.Model
	Phone        string `gorm:"column:phone"`
	Chat         int64  `gorm:"unique,column:chat"`
	TGUser       int  `gorm:"unique,column:tg_user_id"`
	IsAdmin      bool   `gorm:"column:is_admin"`
	RedmineID    int    `gorm:"column:redmine_id"`
	Issues       bool   `gorm:"column:uniqie,column:issue"`
	CurrentIssue int    `gorm:"column:current_issue_id"`
}

type Message struct {
	gorm.Model
	TGUser             int  `gorm:"column:tg_user_id"`
	Status 		       string `gorm:"column:status"`
	Subject		       string `gorm:"column:subject"`
	Phone              string `gorm:"column:phone"`
	IsAdmin			   bool   `gorm:"column:is_admin"`
	SendStatus   	   bool   `gorm:"column:status_send"`
	ApplySendStatus    bool   `gorm:"column:apply_status_send"`
	JSONMessage		   string `gorm:"column:json_message"`
}

func NewDBInstance(dbFile string) *gorm.DB {
	db, err := gorm.Open("sqlite3", dbFile)
	if err != nil {
		log.Panic(err)
	}
	return db
}

func ProcessMigrations(db *gorm.DB) {
	db.AutoMigrate(&User{})
	db.AutoMigrate(&Message{})
}

func FindUsersByPhone(db *gorm.DB, phones []string) (users []*User, err error) {
	err = db.Where("phone IN (?)", phones).Find(&users).Error
	return users, err
}

func GetOrCreateUser(db *gorm.DB, chatID int64, userID int, phone string) (user *User, err error) {
	user = new(User)
	err = db.Where(User{Chat: chatID, Phone: phone}).Last(user).Error
	if err == gorm.ErrRecordNotFound {
		user = &User{
			Chat:    chatID,
			TGUser:  userID,
			Phone:   phone,
			IsAdmin: false,
		}
		db.Create(user)
		return user, nil
	}
	return nil, err
}

func GetOrCreateMessage(db *gorm.DB, userID int, status string, subject string, phone string, isAdmin bool,send_status bool,json_message string) (message *Message, err error) {
	// mu := &sync.Mutex{}
	// globalLock.Lock()
	// defer globalLock.Unlock()
	// // rows, err := db.Begin()
	// if err != nil {
	// 	fmt.Printf("begin. Exec error=%s", err)
	// 	return
	// }
	message = new(Message)
	// defer rows.Commit()
	// err = db.Where(Message{TGUser: userID, Phone: phone}).Last(message).Error
	// if err == gorm.ErrRecordNotFound {
	message = &Message{
		TGUser: userID,
		Status: status,
		Subject: subject,
		Phone: phone,
		IsAdmin: isAdmin,
		SendStatus: send_status,
		ApplySendStatus: false,
		JSONMessage: json_message,
	}
	db.Create(message)
	log.Println("Create New Message")
	return message, nil
	// }
	// return nil, err
}

func GetAdmins(db *gorm.DB) (admins []*User, err error) {
	err = db.Where(User{IsAdmin: true}).Find(&admins).Error
	return admins, err
}

func GetUserByChatID(db *gorm.DB, chat int64) (user *User, err error) {
	err = db.Where(User{Chat: chat}).First(user).Error
	return user, err
}

func GetIssues(db *gorm.DB) (issues []*User, err error) {
	err = db.Where(User{Issues: true}).Find(&issues).Error
	return issues, err
}

func GetCurrentIssueID(db *gorm.DB, current_issue_id int) (issue *User, err error) {
	err = db.Where(User{CurrentIssue: current_issue_id}).First(issue).Error
	return issue, err
}

func UpdateApplySendStatus(db *gorm.DB, message Message) (msg *Message, err error) {
	globalLock := &sync.Mutex{}
	globalLock.Lock()
	defer globalLock.Unlock()
	message.ApplySendStatus = true
	db.Save(message)
	return msg, nil
}