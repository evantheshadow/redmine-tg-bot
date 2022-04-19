package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/imroc/req"
	"github.com/karlseguin/ccache"
)

// RedmineUser ...
type RedmineUser struct {
	Firstname   string `json:"firstname"`
	IconURL     string `json:"icon_url"`
	ID          int    `json:"id"`
	IdentityURL string `json:"identity_url"`
	Lastname    string `json:"lastname"`
	Login       string `json:"login"`
	Mail        string `json:"mail"`
}

// CustomField (старая структура была изменена на новую, смотреть в redmine.go в строке 79-83)
// type CustomField struct {
// 	CustomFieldID   int    `json:"id"`
// 	CustomFieldName string `json:"name"`
// 	Value           string `json:"value"`
// }

// GetNormalizedPhone ...
func (u *RedmineUser) GetNormalizedPhone() string {
	phone := strings.Replace(u.Mail, "+", "", 1)
	phone = strings.Split(phone, "@")[0]
	return phone
}

// FullName ...
func (u *RedmineUser) FullName() string {
	fullname := fmt.Sprintf("%s %s", u.Firstname, u.Lastname)
	return fullname
}

// Priority ...
type Priority struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Project ...
type Project struct {
	CreatedOn   string `json:"created_on"`
	Description string `json:"description"`
	Homepage    string `json:"homepage"`
	ID          int    `json:"id"`
	Identifier  string `json:"identifier"`
	Name        string `json:"name"`
}

// Status ...
type Status struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Tracker ...
type Tracker struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Issue ...
type Issue struct {
	Assignee          RedmineUser `json:"assignee"`
	Author            RedmineUser `json:"author"`
	ClosedOn          string      `json:"closed_on"`
	CreatedOn         string      `json:"created_on"`
	CustomFieldValues []struct {
		ID    int    `json:"id"`
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"custom_fields"`
	Description    string        `json:"description"`
	DoneRatio      int           `json:"done_ratio"`
	DueDate        string        `json:"due_date"`
	EstimatedHours interface{}   `json:"estimated_hours"`
	ID             int           `json:"id"`
	IsPrivate      bool          `json:"is_private"`
	LockVersion    int           `json:"lock_version"`
	ParentID       interface{}   `json:"parent_id"`
	Priority       Priority      `json:"priority"`
	Project        Project       `json:"project"`
	RootID         int           `json:"root_id"`
	StartDate      string        `json:"start_date"`
	Status         Status        `json:"status"`
	Subject        string        `json:"subject"`
	Tracker        Tracker       `json:"tracker"`
	UpdatedOn      string        `json:"updated_on"`
	Watchers       []RedmineUser `json:"watchers"`
}

// Detail ...
type Detail struct {
	ID       int         `json:"id"`
	OldValue interface{} `json:"old_value"`
	PropKey  interface{} `json:"prop_key"`
	Property interface{} `json:"property"`
	Value    interface{} `json:"value"`
}

// Journal ...
type Journal struct {
	Author       RedmineUser `json:"author"`
	CreatedOn    string      `json:"created_on"`
	Details      []Detail    `json:"details"`
	ID           int         `json:"id"`
	Notes        string      `json:"notes"`
	PrivateNotes bool        `json:"private_notes"`
}

//File attachments...
type Attachments struct {
	File Issue `json:"issue"`
}

// Payload ...
type Payload struct {
	Action  string  `json:"action"`
	Issue   Issue   `json:"issue"`
	Journal Journal `json:"journal"`
	URL     string  `json:"url"`
}

func (p *Payload) ActionName() string {
	switch p.Action {
	case "opened":
		return "открыта"
	case "updated":
		return "обновлена"
	default:
		return p.Action
	}
}

// RedmineRequest ...
type RedmineRequest struct {
	Payload Payload `json:"payload"`
}

// GetUserPhones ...
func (rq *RedmineRequest) GetUserPhones() (phones []string) {
	var _phones []string
	if rq.Payload.Action == "updated" {
		_phones = append(_phones, rq.Payload.Issue.Author.GetNormalizedPhone())
	}
	_phones = append(_phones, rq.Payload.Issue.Assignee.GetNormalizedPhone())
	if rq.Payload.Action == "updated" {
		journalAuthor := rq.Payload.Journal.Author.GetNormalizedPhone()
		for _, phone := range _phones {
			if phone != journalAuthor {
				phones = append(phones, phone)
			}
		}
	} else {
		phones = append(phones, _phones...)
	}

	for _, user := range rq.Payload.Issue.Watchers {
		phones = append(phones, user.GetNormalizedPhone())
	}
	return phones
}

// RedmineClient ...
type RedmineClient struct {
	config *Config
	cache  *ccache.Cache
}

// NewRedmineClient - Function for create RedmineClient instance
func NewRedmineClient(config Config) *RedmineClient {
	cache := ccache.New(ccache.Configure())
	return &RedmineClient{
		config: &config,
		cache:  cache,
	}
}

func (rc *RedmineClient) makeRequest(method, url string, header req.Header, params req.Param) (res *req.Resp, err error) {
	_header := req.Header{
		"X-Redmine-API-Key": rc.config.RedmineToken,
	}
	for k, v := range header {
		_header[k] = v
	}

	if rc.config.Debug == "true" {
		req.Debug = true
	}

	switch method {
	case "POST":
		res, err = req.Post(url, _header, req.BodyJSON(params))
	case "GET":
		res, err = req.Get(url, _header, params)
	case "PUT":
		res, err = req.Put(url, _header, req.BodyJSON(params))
	default:
		return nil, errors.New("Method param is required")
	}
	if err != nil {
		return nil, err
	}

	return res, nil
}

// IssueStatusesResponse ...
type IssueStatusesResponse struct {
	Statuses []Status `json:"issue_statuses"`
}

// GetIssueStatuses - Get issue status types
func (rc *RedmineClient) GetIssueStatuses() (statuses *IssueStatusesResponse, err error) {
	apiURL := rc.config.RedmineAPIHost + "issue_statuses.json"

	cached := rc.cache.Get(apiURL)
	if cached != nil {
		statuses = cached.Value().(*IssueStatusesResponse)
		return statuses, nil
	}

	res, err := rc.makeRequest("GET", apiURL, nil, nil)
	if err != nil {
		return nil, err
	}

	statuses = new(IssueStatusesResponse)
	err = res.ToJSON(statuses)
	if err != nil {
		return nil, err
	}

	rc.cache.Set(apiURL, statuses, 60*time.Minute)

	return statuses, nil
}

// IssuePrioritiesResponse ...
type IssuePrioritiesResponse struct {
	Priorities []Priority `json:"issue_priorities"`
}

// GetIssuePriorities - Get issue priority types
func (rc *RedmineClient) GetIssuePriorities() (priorities *IssuePrioritiesResponse, err error) {
	apiURL := rc.config.RedmineAPIHost + "enumerations/issue_priorities.json"

	cached := rc.cache.Get(apiURL)
	if cached != nil {
		priorities = cached.Value().(*IssuePrioritiesResponse)
		return priorities, nil
	}

	res, err := rc.makeRequest("GET", apiURL, nil, nil)
	if err != nil {
		return nil, err
	}

	priorities = new(IssuePrioritiesResponse)
	err = res.ToJSON(priorities)
	if err != nil {
		return nil, err
	}

	rc.cache.Set(apiURL, priorities, 60*time.Minute)

	return priorities, nil
}

// CustomFieldsResponse ...
type CustomFieldsResponse struct {
	CustomFields []struct {
		ID             int    `json:"id"`
		Name           string `json:"name"`
		CustomizedType string `json:"customized_type"`
	} `json:"custom_fields"`
}

// GetCustomFields - Get all custom fields names
func (rc *RedmineClient) GetCustomFields() (fields *CustomFieldsResponse, err error) {
	apiURL := rc.config.RedmineAPIHost + "custom_fields.json"

	cached := rc.cache.Get(apiURL)
	if cached != nil {
		fields = cached.Value().(*CustomFieldsResponse)
		return fields, nil
	}

	res, err := rc.makeRequest("GET", apiURL, nil, nil)
	if err != nil {
		return nil, err
	}

	fields = new(CustomFieldsResponse)
	err = res.ToJSON(fields)
	if err != nil {
		return nil, err
	}

	rc.cache.Set(apiURL, fields, 60*time.Minute)

	return fields, nil
}

type UsersResponse struct {
	Users      []RedmineUser `json:"users"`
	TotalCount int           `json:"total_count"`
	Limit      int           `json:"limit"`
	Offset     int           `json:"offset"`
}

func (rc *RedmineClient) GetUsers() (users *UsersResponse, err error) {
	var tempUsers *UsersResponse

	apiURL := rc.config.RedmineAPIHost + "users.json"

	cached := rc.cache.Get(apiURL)
	if cached != nil {
		users = cached.Value().(*UsersResponse)
		return users, nil
	}

	offset := 0
	limit := 25
	batch := 25

	users = new(UsersResponse)

	for idx := 0; idx < 100; idx++ {
		url := fmt.Sprintf("%s?limit=%d&offset=%d", apiURL, limit, offset)
		res, err := rc.makeRequest("GET", url, nil, nil)
		if err != nil {
			return nil, err
		}

		tempUsers = new(UsersResponse)
		err = res.ToJSON(tempUsers)
		if err != nil {
			return nil, err
		}

		if len(tempUsers.Users) == 0 {
			break
		}

		users.Users = append(users.Users, tempUsers.Users...)

		offset += batch
	}

	rc.cache.Set(apiURL, users, 60*time.Minute)

	return users, nil
}

type MembershipsResponse struct {
	Users []struct {
		User RedmineUser `json:"user"`
	} `json:"memberships"`
	TotalCount int `json:"total_count"`
	Limit      int `json:"limit"`
	Offset     int `json:"offset"`
}

func (rc *RedmineClient) GetMembershipsByProject(id int) (memberships *MembershipsResponse, err error) {
	var tempMembers *MembershipsResponse

	apiURL := rc.config.RedmineAPIHost + "projects/%d/memberships.json"
	apiURL = fmt.Sprintf(apiURL, id)

	cached := rc.cache.Get(apiURL)
	if cached != nil {
		memberships = cached.Value().(*MembershipsResponse)
		return memberships, nil
	}

	offset := 0
	limit := 25
	batch := 25

	memberships = new(MembershipsResponse)

	for idx := 0; idx < 100; idx++ {
		url := fmt.Sprintf("%s?limit=%d&offset=%d", apiURL, limit, offset)
		res, err := rc.makeRequest("GET", url, nil, nil)
		if err != nil {
			return nil, err
		}

		tempMembers = new(MembershipsResponse)
		err = res.ToJSON(tempMembers)
		if err != nil {
			return nil, err
		}

		if len(tempMembers.Users) == 0 {
			break
		}

		memberships.Users = append(memberships.Users, tempMembers.Users...)

		offset += batch
	}

	rc.cache.Set(apiURL, memberships, 60*time.Minute)

	return memberships, nil
}

// Функция для заполнения массива настраиваемых полей (Его выполнение Вы можете увидеть в main.go,
// функция makeClientRequest)

type CustomFieldIssueResponse struct {
	Issue struct {
		ID                int `json:"id"`
		CustomFieldValues []struct {
			ID    int    `json:"id"`
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"custom_fields"`
	}
}

func (rc *RedmineClient) GetAPIForCustomFields(id int) (customFields *CustomFieldIssueResponse, err error) {
	var tempCustomFields *CustomFieldIssueResponse

	apiURL := rc.config.RedmineAPIHost + "issues/%d.json"
	apiURL = fmt.Sprintf(apiURL, id)

	cached := rc.cache.Get(apiURL)
	if cached != nil {
		customFields = cached.Value().(*CustomFieldIssueResponse)
		return customFields, nil
	}

	resp, err := rc.makeRequest("GET", apiURL, nil, nil)
	if err != nil {
		return nil, err
	}

	tempCustomFields = new(CustomFieldIssueResponse)
	err = resp.ToJSON(tempCustomFields)
	customFields = tempCustomFields
	rc.cache.Set(apiURL, customFields, 60*time.Minute)

	return customFields, nil
}

func (rc *RedmineClient) GetClientDataFromCustomFields(id int) (numPhone string, address string, err error) {
	var tempCustomFields *CustomFieldIssueResponse

	apiURL := rc.config.RedmineAPIHost + "issues/%d.json"
	apiURL = fmt.Sprintf(apiURL, id)

	resp, err := rc.makeRequest("GET", apiURL, nil, nil)
	if err != nil {
		return "", "", err
	}

	tempCustomFields = new(CustomFieldIssueResponse)
	err = resp.ToJSON(tempCustomFields)
	for _, custom_field := range tempCustomFields.Issue.CustomFieldValues {
		if custom_field.ID == 15 {
			address = custom_field.Value;
		}
		if custom_field.ID == 19 {
			numPhone = custom_field.Value;
		}
	}
	return numPhone, address, nil;
}

type UpdateIssueAPIResponse struct {
	Issue struct {
		StatusID  int       `json:"status_id"`
		Notes     string    `json:"notes"`
	} `json:"issue"`
}

func (rc *RedmineClient) UpdateStatusIssue(issueID int, postStatusID int) (newResponse string) {
	updateAPI := new(UpdateIssueAPIResponse)
	url := rc.config.RedmineAPIHost + "issues/%d.json"
	url = fmt.Sprintf(url, issueID)

	updateAPI.Issue.StatusID = postStatusID
	if postStatusID == 5 {
		updateAPI.Issue.Notes = "Заявка была закрыта!"
	} else if postStatusID == 6 {
		updateAPI.Issue.Notes = "Заявка была отклонена!"
	} else if postStatusID == 9 {
		updateAPI.Issue.Notes = "Заявка была подтверждена! Не забудьте связаться с абонентом по заявке."
	}

	param := req.Param{
		"issue": updateAPI.Issue,
	}
	res, err := rc.makeRequest("PUT", url, nil, param)
	if err != nil {
		fmt.Println("JSON Marshal Response Error:",err)
		return ""
	}
	newResponse = res.String()

	return newResponse
}