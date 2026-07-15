package jira

import (
	"encoding/json"
	"net/http"
)

type Credentials struct {
	Username string
	Password string
}

type Client struct {
	baseURL     string
	httpClient  *http.Client
	credentials Credentials
}

type Status struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	CategoryKey     string `json:"-"`
	TransitionCount int    `json:"-"`
}

type User struct {
	AccountID   string `json:"accountId"`
	DisplayName string `json:"displayName"`
}

type Issue struct {
	Key             string
	Summary         string
	DescriptionText string
	Status          Status
	Assignee        *User
	CustomFields    map[string]json.RawMessage
}

type Transition struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	To   Status `json:"to"`
}

type TransitionOption struct {
	ID     string
	Status string
}

type RequiredInput struct {
	Kind    string
	Message string
	Options []TransitionOption
}

type TransitionDecision struct {
	Transition     *Transition
	RequiredInputs []RequiredInput
	Complete       bool
}
