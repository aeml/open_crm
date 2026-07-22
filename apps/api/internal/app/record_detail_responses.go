package app

import (
	moduleactivityfeed "github.com/aeml/open_crm/apps/api/internal/modules/activityfeed"
	modulecompanies "github.com/aeml/open_crm/apps/api/internal/modules/companies"
	modulecontacts "github.com/aeml/open_crm/apps/api/internal/modules/contacts"
)

type contactDetailResponse struct {
	Data struct {
		Contact      modulecontacts.Summary         `json:"contact"`
		Notes        []modulecontacts.NoteEntry     `json:"notes"`
		Tasks        []modulecontacts.TaskEntry     `json:"tasks"`
		Activities   []modulecontacts.ActivityEntry `json:"activities"`
		ActivityMeta moduleactivityfeed.Meta        `json:"activityMeta"`
		NoteMeta     moduleactivityfeed.Meta        `json:"noteMeta"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type companyDetailResponse struct {
	Data struct {
		Company           modulecompanies.Summary         `json:"company"`
		LinkedContacts    []modulecompanies.LinkedContact `json:"linkedContacts"`
		LinkedContactMeta modulecompanies.ListMeta        `json:"linkedContactMeta"`
		Activities        []modulecompanies.ActivityEntry `json:"activities"`
		ActivityMeta      moduleactivityfeed.Meta         `json:"activityMeta"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}
