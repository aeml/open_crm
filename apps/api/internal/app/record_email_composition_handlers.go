package app

import (
	"context"
	"errors"
	"net/http"
	"strings"

	moduleauth "github.com/aeml/open_crm/apps/api/internal/modules/auth"
	modulecustomfields "github.com/aeml/open_crm/apps/api/internal/modules/customfields"
	moduleemailtemplates "github.com/aeml/open_crm/apps/api/internal/modules/emailtemplates"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

var errRecordEmailRecipient = errors.New("record has no eligible email recipient")

type recordEmailComposition struct {
	RecipientContactID int64
	RecipientEmail     string
	Fields             map[string]string
}

type recordEmailPreviewResponse struct {
	Data struct {
		To                    string   `json:"to"`
		Subject               string   `json:"subject"`
		Body                  string   `json:"body"`
		UnresolvedMergeFields []string `json:"unresolvedMergeFields"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

func loadContactEmailComposition(ctx context.Context, organizationID, contactID int64, contacts contactsService, customFields customFieldsService) (recordEmailComposition, error) {
	detail, err := contacts.GetByID(ctx, organizationID, contactID)
	if err != nil {
		return recordEmailComposition{}, err
	}
	to := strings.TrimSpace(detail.Summary.Email)
	if to == "" {
		return recordEmailComposition{}, errRecordEmailRecipient
	}
	definitions, _, err := loadRecordEmailCustomFieldDefinitions(ctx, organizationID, customFields)
	if err != nil {
		return recordEmailComposition{}, err
	}
	fields := contactMergeFields(detail)
	moduleemailtemplates.AddCustomMergeFields(fields, "contact", definitions, detail.Summary.CustomFields)
	return recordEmailComposition{RecipientContactID: contactID, RecipientEmail: to, Fields: fields}, nil
}

func loadCompanyEmailComposition(ctx context.Context, organizationID, companyID, requestedContactID int64, companies companiesService, contacts contactsService, customFields customFieldsService) (recordEmailComposition, error) {
	detail, err := companies.GetByID(ctx, organizationID, companyID)
	if err != nil {
		return recordEmailComposition{}, err
	}
	recipient, ok := companyEmailRecipient(detail, requestedContactID)
	if !ok {
		return recordEmailComposition{}, errRecordEmailRecipient
	}
	var contactValues modulecustomfields.Values
	if contacts != nil {
		contact, contactErr := contacts.GetByID(ctx, organizationID, recipient.ID)
		if contactErr != nil {
			return recordEmailComposition{}, contactErr
		}
		contactValues = contact.Summary.CustomFields
	}
	contactDefinitions, companyDefinitions, err := loadRecordEmailCustomFieldDefinitions(ctx, organizationID, customFields)
	if err != nil {
		return recordEmailComposition{}, err
	}
	fields := companyMergeFields(detail, recipient)
	moduleemailtemplates.AddCustomMergeFields(fields, "contact", contactDefinitions, contactValues)
	moduleemailtemplates.AddCustomMergeFields(fields, "company", companyDefinitions, detail.Summary.CustomFields)
	return recordEmailComposition{RecipientContactID: recipient.ID, RecipientEmail: recipient.Email, Fields: fields}, nil
}

func loadDealEmailComposition(ctx context.Context, organizationID, dealID, requestedContactID int64, deals dealsService, contacts contactsService, companies companiesService, customFields customFieldsService) (recordEmailComposition, error) {
	detail, err := deals.GetByID(ctx, organizationID, dealID)
	if err != nil {
		return recordEmailComposition{}, err
	}
	if detail.Summary.PrimaryContactID <= 0 || (requestedContactID > 0 && requestedContactID != detail.Summary.PrimaryContactID) {
		return recordEmailComposition{}, errRecordEmailRecipient
	}
	contact, err := contacts.GetByID(ctx, organizationID, detail.Summary.PrimaryContactID)
	if err != nil {
		return recordEmailComposition{}, err
	}
	to := strings.TrimSpace(contact.Summary.Email)
	if to == "" {
		return recordEmailComposition{}, errRecordEmailRecipient
	}
	contactDefinitions, companyDefinitions, err := loadRecordEmailCustomFieldDefinitions(ctx, organizationID, customFields)
	if err != nil {
		return recordEmailComposition{}, err
	}
	fields := dealMergeFields(detail, contact)
	moduleemailtemplates.AddCustomMergeFields(fields, "contact", contactDefinitions, contact.Summary.CustomFields)
	if detail.Summary.CompanyID > 0 && companies != nil {
		company, companyErr := companies.GetByID(ctx, organizationID, detail.Summary.CompanyID)
		if companyErr != nil {
			return recordEmailComposition{}, companyErr
		}
		moduleemailtemplates.AddCustomMergeFields(fields, "company", companyDefinitions, company.Summary.CustomFields)
	} else {
		moduleemailtemplates.AddCustomMergeFields(fields, "company", companyDefinitions, nil)
	}
	return recordEmailComposition{RecipientContactID: detail.Summary.PrimaryContactID, RecipientEmail: to, Fields: fields}, nil
}

func loadRecordEmailCustomFieldDefinitions(ctx context.Context, organizationID int64, customFields customFieldsService) ([]modulecustomfields.Definition, []modulecustomfields.Definition, error) {
	if customFields == nil {
		return nil, nil, nil
	}
	contactDefinitions, err := customFields.List(ctx, organizationID, "contact", false)
	if err != nil {
		return nil, nil, err
	}
	companyDefinitions, err := customFields.List(ctx, organizationID, "company", false)
	if err != nil {
		return nil, nil, err
	}
	return contactDefinitions, companyDefinitions, nil
}

func handlePreviewRecordEmail(auth authService, contacts contactsService, companies companiesService, deals dealsService, customFields customFieldsService, entityType string, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if contacts == nil || companies == nil || deals == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Email preview service unavailable")
		return
	}
	entityID, ok := parsePathInt64(w, r, entityType+"ID")
	if !ok {
		return
	}
	var request sendRecordEmailRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	composition, err := loadRecordEmailComposition(r.Context(), state, entityType, entityID, request.ContactID, contacts, companies, deals, customFields)
	if err != nil {
		writeRecordEmailCompositionError(w, requestID, err)
		return
	}
	subject := strings.TrimSpace(moduleemailtemplates.Render(request.Subject, composition.Fields))
	body := strings.TrimSpace(moduleemailtemplates.Render(request.Body, composition.Fields))
	if subject == "" || body == "" {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Subject and body are required")
		return
	}
	response := recordEmailPreviewResponse{}
	response.Data.To = composition.RecipientEmail
	response.Data.Subject = subject
	response.Data.Body = body
	response.Data.UnresolvedMergeFields = moduleemailtemplates.UnresolvedTokens(subject, body)
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func loadRecordEmailComposition(ctx context.Context, state moduleauth.SessionState, entityType string, entityID, requestedContactID int64, contacts contactsService, companies companiesService, deals dealsService, customFields customFieldsService) (recordEmailComposition, error) {
	switch entityType {
	case "contact":
		return loadContactEmailComposition(ctx, state.Organization.ID, entityID, contacts, customFields)
	case "company":
		return loadCompanyEmailComposition(ctx, state.Organization.ID, entityID, requestedContactID, companies, contacts, customFields)
	case "deal":
		return loadDealEmailComposition(ctx, state.Organization.ID, entityID, requestedContactID, deals, contacts, companies, customFields)
	default:
		return recordEmailComposition{}, errors.New("unsupported record email entity")
	}
}

func writeRecordEmailCompositionError(w http.ResponseWriter, requestID string, err error) {
	if writeResourceNotFound(w, requestID, err) {
		return
	}
	if errors.Is(err, errRecordEmailRecipient) {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "The selected record has no eligible email recipient")
		return
	}
	platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to prepare email content")
}
