package app

import moduleemailmessages "github.com/aeml/open_crm/apps/api/internal/modules/emailmessages"

type sharedInboxMessagesListResponse struct {
	Data struct {
		Messages []emailMessageView                      `json:"messages"`
		Meta     moduleemailmessages.SharedInboxPageMeta `json:"meta"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}
