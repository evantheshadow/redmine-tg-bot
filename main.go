package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
	"encoding/json"
	"bytes"
	"io/ioutil"
	"strconv"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/labstack/echo"
)

func initTgBot(config Config) (*tgbotapi.BotAPI, tgbotapi.UpdatesChannel) {
	var (
		bot *tgbotapi.BotAPI
		err error
	)

	if (config.Proxy != ProxyConfig{}) {
		httpClient := getProxyClient(
			config.Proxy.Scheme,
			config.Proxy.Host,
			config.Proxy.Port,
			config.Proxy.User,
			config.Proxy.Password,
		)
		bot, err = tgbotapi.NewBotAPIWithClient(config.TgToken, httpClient)
	} else {
		bot, err = tgbotapi.NewBotAPI(config.TgToken)
	}
	if err != nil {
		log.Fatal(err)
	}

	if config.Debug == "true" {
		bot.Debug = true
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := bot.GetUpdatesChan(u)

	if err != nil {
		log.Fatal(err)
	}

	return bot, updates
}

func clientMakeRequest(message *RedmineRequest,handler *IssuesHandler) {
	jsonStr, err := json.Marshal(message)
	if err != nil {
		fmt.Println(err)
		return
	}

	payload := bytes.NewBuffer(jsonStr)
	byteValue, _ := ioutil.ReadAll(payload)
    var req RedmineRequest

	json.Unmarshal([]byte(byteValue), &req)
	custom_fields, err := handler.redmine.GetAPIForCustomFields(req.Payload.Issue.ID) 
	// Запуск тестового скрипта для проверки, есть ли в API настраиваемые поля
	if err != nil {
		fmt.Println("Custom Fields Error: ", err)
	} else {
		req.Payload.Issue.CustomFieldValues = custom_fields.Issue.CustomFieldValues
	}
	fmt.Println("Custom Fields:", req.Payload.Issue.CustomFieldValues)
	var str_number string
	var isGetNotification = false;
	for _, custom_field := range req.Payload.Issue.CustomFieldValues {
		if custom_field.ID == 19 {
			buffer_number := []rune(custom_field.Value)
			buffer_number[0] = '7'
			str_number = string(buffer_number)
		}
		if (custom_field.ID == 23) && (custom_field.Value == strconv.Itoa(1)) {
			isGetNotification = true;
		}
	}
	if (isGetNotification == true) && ((req.Payload.Action == "opened") || ((req.Payload.Issue.Status.ID == 5 || req.Payload.Issue.Status.ID == 6 || req.Payload.Issue.Status.ID == 9) && (req.Payload.Action == "updated"))) {
		var phones = []string{str_number}
		users, err := FindUsersByPhone(handler.db, phones)
		if err != nil {
			fmt.Println("Error Phone Number:", err)
		}
		uniqueUsers := make(map[uint]*User)
		for _, user := range users {
			if _, ok := uniqueUsers[user.ID]; !ok {
				uniqueUsers[user.ID] = user
			}
		}
		var resultMsg string
		if (req.Payload.Issue.Status.ID == 1) {
			resultMsg = "Ваша заявка №"+strconv.Itoa(req.Payload.Issue.ID)+" была создана!\n\nСтатус: Открыта\nУслуга: Поверка счетчиков\nНомер телефона: +"+str_number+"\n\nСкоро Ваша заявка будет рассмотрена специалистом!"
		}
		if (req.Payload.Issue.Status.ID == 5) {
			resultMsg = "Ваша заявка №"+strconv.Itoa(req.Payload.Issue.ID)+" была закрыта!\n\nСтатус: Закрыта\nУслуга: Поверка счетчиков\nНомер телефона: +"+str_number+"\n\nВаша заявка была сделана специалистом!\nЕсли Вы остались недовольны предоставленными услугами - позвоните нам."
		}
		if (req.Payload.Issue.Status.ID == 6) {
			resultMsg = "Ваша заявка №"+strconv.Itoa(req.Payload.Issue.ID)+" была отклонена!\n\nСтатус: Отклонена\nУслуга: Поверка счетчиков\nНомер телефона: +"+str_number+"\n\nВаша заявка была отклонена специалистом!\nПопробуйте назначить другое время."
		}
		if (req.Payload.Issue.Status.ID == 9) {
			resultMsg = "Ваша заявка №"+strconv.Itoa(req.Payload.Issue.ID)+" была подтверждена!\n\nСтатус: Подтверждена\nУслуга: Поверка счетчиков\nНомер телефона: +"+str_number+"\n\nВаша заявка была подтверждена специалистом!\nОжидайте мастера в назначенное Вами время."
		}
		jsonStr, err := json.Marshal(req)
		if err != nil {
			fmt.Println(err)
		}
		respData := bytes.NewBuffer(jsonStr)
		jsonToModel := respData.String()
		for _, user := range uniqueUsers {
			if err != nil {
				fmt.Println("Error Create Message:", err)
			}
			message := tgbotapi.NewMessage(user.Chat, resultMsg)
			_, err := handler.bot.Send(message)
			jsMsg, err := GetOrCreateMessage(handler.db, user.TGUser, req.Payload.Issue.Status.Name, req.Payload.Issue.Subject, str_number, false, true, jsonToModel)
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
				fmt.Println("Error Send Notification Client",err)
			} else {
				_, err := UpdateApplySendStatus(handler.db, reqMsg)			
				if err != nil {
					fmt.Println("USER - Update is failed!", err)
				}
			}
		}
	}
	return
}

func initHTTPServer(config Config, issuesUpdates chan RedmineRequest, handler *IssuesHandler) (*echo.Echo, string) {
	e := echo.New()
	e.POST("/webhook", func(c echo.Context) error {
		redmineRequest := new(RedmineRequest)
		if err := c.Bind(redmineRequest); err != nil {
			return err
		}
		clientMakeRequest(redmineRequest,handler)
		issuesUpdates <- *redmineRequest

		return c.NoContent(http.StatusOK)
	})
	if config.Debug == "true" {
		e.Debug = true
	}
	bindURL := fmt.Sprintf("%s:%d", config.WebhookHost, config.WebhookPort)
	return e, bindURL
}

func main() {
	configFile := flag.String("config", "./config.toml", "Path to config file")
	flag.Parse()

	config := parseConfig(*configFile)

	issueUpdates := make(chan RedmineRequest, config.QueueSize)
	defer close(issueUpdates)

	db := NewDBInstance(config.DbFile)
	var globalLock sync.Mutex
	globalLock.Lock()
	defer globalLock.Unlock()
	ProcessMigrations(db)

	bot, tgUpdates := initTgBot(config)
	redmine := NewRedmineClient(config)
	handler := NewIssuesHandler(config, bot, redmine, issueUpdates)
	server, bindURL := initHTTPServer(config, issueUpdates, handler)
	authHandler := NewAuthHandler(config, db, bot)

	go func() {
		server.Logger.Fatal(server.Start(bindURL))
	}()
	go func() {
		for update := range tgUpdates {
			if update.CallbackQuery != nil{
				postStatusID,err := strconv.Atoi((string([]rune(update.CallbackQuery.Data)[0])))
				issueID,err := strconv.Atoi((string([]rune(update.CallbackQuery.Data)[1:])))
				if err != nil {
					log.Fatal(err)
				}
				res := redmine.UpdateStatusIssue(issueID, postStatusID)
				fmt.Println(res)
			}
			if update.Message == nil {
				continue
			}
			authHandler.Authenticate(update.Message)
		}
	}()
	go handler.Run()

	quit := make(chan os.Signal)
	defer close(quit)
	signal.Notify(quit, os.Interrupt)

	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bot.StopReceivingUpdates()
	handler.Stop()
	if err := server.Shutdown(ctx); err != nil {
		server.Logger.Fatal(err)
	}
	os.Exit(0)
}
