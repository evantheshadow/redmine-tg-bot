package main

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"encoding/json"
	"io/ioutil"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/jinzhu/gorm"
)

var (
	ErrValueNotFound    = errors.New("Value not found")
	ErrFieldNotDeclared = errors.New("Field not declared")
)

// IssuesHandler ...
type IssuesHandler struct {
	config    Config
	bot       *tgbotapi.BotAPI
	redmine   *RedmineClient
	updates   chan RedmineRequest
	closeChan chan interface{}
	db        *gorm.DB
}

// NewIssuesHandler ...
func NewIssuesHandler(config Config, bot *tgbotapi.BotAPI, redmine *RedmineClient, updates chan RedmineRequest) *IssuesHandler {
	handler := &IssuesHandler{
		config:    config,
		bot:       bot,
		redmine:   redmine,
		updates:   updates,
		closeChan: make(chan interface{}),
		db:        NewDBInstance(config.DbFile),
	}
	return handler
}

// Run ...
func (h *IssuesHandler) Run() {
	for {
		select {
		case issue := <-h.updates:
			h.sendNotifications(issue)
		case <-h.closeChan:
			return
		default:
			time.Sleep(300 * time.Millisecond)
		}
	}
}

type JournalDetail struct {
	Name       string
	DetailType int
	OldValue   string
	NewValue   string
}

type TemplateData struct {
	Author      string
	Action      string
	Project     string
	IssueID     int
	Subject     string
	Description string
	PhoneNumber string
	Address		string
	Status      string
	Assignee    string
	Notes       string
	Journals    []JournalDetail
}

func (h *IssuesHandler) getIssueStatusName(value string) (string, error) {
	statuses, err := h.redmine.GetIssueStatuses()
	if err != nil {
		return "", err
	}
	for _, v := range statuses.Statuses {
		if strconv.Itoa(v.ID) == value {
			return v.Name, nil
		}
	}
	return "", ErrValueNotFound
}

func (h *IssuesHandler) getIssuePriorityName(value string) (string, error) {
	priorities, err := h.redmine.GetIssuePriorities()
	if err != nil {
		return "", err
	}
	for _, v := range priorities.Priorities {
		if strconv.Itoa(v.ID) == value {
			return v.Name, nil
		}
	}
	return "", ErrValueNotFound
}

func (h *IssuesHandler) getUserName(value string) (string, error) {
	users, err := h.redmine.GetUsers()
	if err != nil {
		return "", err
	}
	for _, v := range users.Users {
		if strconv.Itoa(v.ID) == value {
			return v.FullName(), nil
		}
	}
	return "", ErrValueNotFound
}

func (h *IssuesHandler) getFieldName(detail Detail) (string, error) {
	var detailName string
	switch detail.PropKey {
	case "status_id":
		detailName = "Статус"
	case "priority_id":
		detailName = "Приоритет"
	case "due_date":
		detailName = "Дата выполнения"
	case "assigned_to_id":
		detailName = "Назначена"
	case "done_ratio":
		detailName = "Готовность"
	case "subject":
		detailName = "Тема"
	default:
		return "", ErrFieldNotDeclared
	}
	return detailName, nil
}

func (h *IssuesHandler) getDetailType(detail Detail) int {
	switch {
	case detail.OldValue != nil && detail.Value != nil:
		return 1
	case detail.Value == nil:
		return 2
	case detail.OldValue == nil:
		return 3
	}
	return 0
}

func (h *IssuesHandler) fillJournalDetails(details []Detail, data *TemplateData) error {
	for _, detail := range details {
		if detail.Property == "cf" { // Skip custom fields
			continue
		}

		oldValue := fmt.Sprintf("%v", detail.OldValue)
		newValue := fmt.Sprintf("%v", detail.Value)

		detailType := h.getDetailType(detail)
		fieldName, err := h.getFieldName(detail)
		if err == ErrFieldNotDeclared {
			continue
		}
		if detail.PropKey == "status_id" {
			oldValue, err = h.getIssueStatusName(oldValue)
			if err == ErrValueNotFound {
				return err
			}
			newValue, err = h.getIssueStatusName(newValue)
			if err == ErrValueNotFound {
				return err
			}
		} else if detail.PropKey == "priority_id" {
			oldValue, err = h.getIssuePriorityName(oldValue)
			if err == ErrValueNotFound {
				return err
			}
			newValue, err = h.getIssuePriorityName(newValue)
			if err == ErrValueNotFound {
				return err
			}
		} else if detail.PropKey == "assigned_to_id" {
			oldValue, err = h.getUserName(oldValue)
			if err == ErrValueNotFound {
				return err
			}
			newValue, err = h.getUserName(newValue)
			if err == ErrValueNotFound {
				return err
			}
		}

		journalDetail := JournalDetail{
			OldValue:   oldValue,
			NewValue:   newValue,
			Name:       fieldName,
			DetailType: detailType,
		}
		data.Journals = append(data.Journals, journalDetail)
	}
	return nil
}

func (h *IssuesHandler) buildKeyboard(issue RedmineRequest) *tgbotapi.InlineKeyboardMarkup {
	urlButtons := []tgbotapi.InlineKeyboardButton{}
	urlButtons = append(urlButtons, tgbotapi.NewInlineKeyboardButtonURL(
		"Открыть заявку в браузере",
		fmt.Sprintf("%sissues/%d", h.config.RedmineHost, issue.Payload.Issue.ID),
	))
	if issue.Payload.Issue.Status.ID == 1 {
		urlButtons = append(urlButtons, tgbotapi.NewInlineKeyboardButtonData(
			"Подтвердить заявку",
			strconv.Itoa(9)+strconv.Itoa(issue.Payload.Issue.ID),
		))
		urlButtons = append(urlButtons, tgbotapi.NewInlineKeyboardButtonData(
			"Отклонить заявку",
			strconv.Itoa(6)+strconv.Itoa(issue.Payload.Issue.ID),
		))
	}
	if issue.Payload.Issue.Status.ID == 9 {
		urlButtons = append(urlButtons, tgbotapi.NewInlineKeyboardButtonData(
			"Закрыть заявку",
			strconv.Itoa(5)+strconv.Itoa(issue.Payload.Issue.ID),
		))
	}
	Kb := tgbotapi.NewInlineKeyboardMarkup(urlButtons)
	return &Kb
}

func (h *IssuesHandler) getAdminsForNotifications(projectID int) (admins []*User, err error) {
	_admins, err := GetAdmins(h.db)
	if err != nil {
		return nil, err
	}

	_members, err := h.redmine.GetMembershipsByProject(projectID)

	if err != nil {
		return nil, err
	}

	members := make(map[int]RedmineUser)
	for _, member := range _members.Users {
		members[member.User.ID] = member.User
	}

	for _, admin := range _admins {
		if _, ok := members[int(admin.RedmineID)]; ok {
			admins = append(admins, admin)
		}
	}

	return admins, nil
}

func (h *IssuesHandler) renderTemplate(data *TemplateData) (notification string, err error) {
	var t bytes.Buffer

	tmpl, err := getTemplate(h.config)
	if err != nil {
		fmt.Println(err)
		return
	}
	tmpl.Execute(&t, data)
	notification = t.String()

	return notification, nil
}

func (h *IssuesHandler) sendNotifications(issue RedmineRequest) {
	journal := issue.Payload.Journal
	data := TemplateData{
		Project:     issue.Payload.Issue.Project.Name,
		IssueID:     issue.Payload.Issue.ID,
		Subject:     issue.Payload.Issue.Subject,
		Assignee:    issue.Payload.Issue.Assignee.FullName(),
		Description: issue.Payload.Issue.Description,
		Status:      issue.Payload.Issue.Status.Name,
		Action:      issue.Payload.ActionName(),
	}

	if issue.Payload.Action == "updated" {
		data.Notes = journal.Notes
		data.Author = journal.Author.FullName()
	}

	numPhone, address, errNum := h.redmine.GetClientDataFromCustomFields(issue.Payload.Issue.ID)
	if errNum != nil {
		fmt.Println("Num Phone Error: ", errNum)
		return
	}
	data.PhoneNumber = numPhone;
	data.Address = address;

	err := h.fillJournalDetails(journal.Details, &data)
	if err != nil {
		fmt.Println(err)
		return
	}

	resultMsg, err := h.renderTemplate(&data)
	if err != nil {
		fmt.Println(err)
		return
	}

	phones := issue.GetUserPhones()
	users, err := FindUsersByPhone(h.db, phones)
	if err == gorm.ErrRecordNotFound {
		fmt.Println(err)
		return
	}
	if issue.Payload.Action == "opened" {
		admins, err := h.getAdminsForNotifications(issue.Payload.Issue.Project.ID)
		if err != nil {
			fmt.Println(err)
			return
		}
		users = append(users, admins...)
	}

	uniqueUsers := make(map[uint]*User)
	for _, user := range users {
		if _, ok := uniqueUsers[user.ID]; !ok {
			uniqueUsers[user.ID] = user
		}
	}

	kb := h.buildKeyboard(issue)
	for _, user := range uniqueUsers {
		jsonStr, err := json.Marshal(issue)
		if err != nil {
			fmt.Println(err)
		}
		respData := bytes.NewBuffer(jsonStr)
		jsonToModel := respData.String()
		message := tgbotapi.NewMessage(user.Chat, resultMsg)
		message.ParseMode = "html"
		message.ReplyMarkup = kb
		h.bot.Send(message)
		jsMsg, err := GetOrCreateMessage(h.db, user.TGUser, issue.Payload.Issue.Status.Name, issue.Payload.Issue.Subject, user.Phone, true, true, jsonToModel)
		jsonStrMsg, err := json.Marshal(jsMsg)
		if err != nil {
			fmt.Println(err)
			return
		}
		jMsg := bytes.NewBuffer(jsonStrMsg)
		byteValue, _ := ioutil.ReadAll(jMsg)
	
		var reqMsg Message
	
		json.Unmarshal([]byte(byteValue), &reqMsg)
		if err != nil {
			fmt.Println("Error Notification:",err)
		} else {
			_, err := UpdateApplySendStatus(h.db, reqMsg)			
			if err != nil {
				fmt.Println(">> ADMIN - Update is failed:", err)
			}
		}
	}
	return
}

// Stop ...
func (h *IssuesHandler) Stop() {
	h.closeChan <- 0
}

type AuthHandler struct {
	config Config
	db     *gorm.DB
	bot    *tgbotapi.BotAPI
}

func NewAuthHandler(config Config, db *gorm.DB, bot *tgbotapi.BotAPI) *AuthHandler {
	return &AuthHandler{
		config: config,
		bot:    bot,
		db:     db,
	}
}

func (ah *AuthHandler) Authenticate(message *tgbotapi.Message) {
	chatID := message.Chat.ID
	userID := message.From.ID
	if message.IsCommand() && message.Command() == "start" {
		kb := tgbotapi.NewReplyKeyboard(
			tgbotapi.NewKeyboardButtonRow(
				tgbotapi.NewKeyboardButtonContact("Авторизоваться"),
			),
		)
		newMessage := tgbotapi.NewMessage(
			message.Chat.ID,
			"Для авторизации необходим номер телефона.\n\nДля того, чтобы отправить его, необходимо нажать на кнопку 'Авторизация'.",
		)
		newMessage.ReplyMarkup = kb
		ah.bot.Send(newMessage)
		return
	}
	if message.Contact != nil && message.Contact.UserID == userID {
		phoneNumber := strings.ReplaceAll(message.Contact.PhoneNumber, "+", "")
		_, err := GetOrCreateUser(ah.db, chatID, userID, phoneNumber)
		if err != nil {
			fmt.Println(err)
			return
		}
		newMessage := tgbotapi.NewMessage(
			chatID,
			"Вы успешно авторизованы.\n\nТеперь Вы сможете получать уведомления по заявкам и отправлять фото о проделаной работе!",
		)
		newMessage.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
		ah.bot.Send(newMessage)
		return
	}
}
