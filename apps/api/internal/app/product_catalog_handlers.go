package app

import (
	"errors"
	"net/http"
	"strconv"

	moduleproductcatalog "github.com/aeml/open_crm/apps/api/internal/modules/productcatalog"
	platformweb "github.com/aeml/open_crm/apps/api/internal/platform/web"
)

type productCatalogListResponse struct {
	Data struct {
		Items []moduleproductcatalog.Item `json:"items"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type productCatalogItemResponse struct {
	Data struct {
		Item moduleproductcatalog.Item `json:"item"`
	} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

type productCatalogRequest struct {
	Name        string `json:"name"`
	SKU         string `json:"sku"`
	Description string `json:"description"`
	ItemType    string `json:"itemType"`
	UnitPrice   string `json:"unitPrice"`
	Currency    string `json:"currency"`
	UnitName    string `json:"unitName"`
	IsActive    *bool  `json:"isActive"`
}

func handleListProductCatalogItems(auth authService, catalog productCatalogService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgMember(auth, w, r)
	if !ok {
		return
	}
	if catalog == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Product catalog service unavailable")
		return
	}

	items, err := catalog.ListByOrganization(r.Context(), state.Organization.ID)
	if err != nil {
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to load product catalog")
		return
	}

	response := productCatalogListResponse{}
	response.Data.Items = items
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleCreateProductCatalogItem(auth authService, catalog productCatalogService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if catalog == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Product catalog service unavailable")
		return
	}

	var request productCatalogRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	item, err := catalog.Create(r.Context(), state.Organization.ID, state.User.ID, productCatalogInput(request))
	if err != nil {
		writeProductCatalogError(w, requestID, err)
		return
	}

	response := productCatalogItemResponse{}
	response.Data.Item = item
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusCreated, response)
}

func handleUpdateProductCatalogItem(auth authService, catalog productCatalogService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if catalog == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Product catalog service unavailable")
		return
	}

	itemID, ok := parseProductCatalogItemID(w, r, requestID)
	if !ok {
		return
	}
	var request productCatalogRequest
	if !decodeJSONRequest(w, r, requestID, &request) {
		return
	}
	item, err := catalog.Update(r.Context(), state.Organization.ID, itemID, state.User.ID, productCatalogInput(request))
	if err != nil {
		writeProductCatalogError(w, requestID, err)
		return
	}

	response := productCatalogItemResponse{}
	response.Data.Item = item
	response.Meta.RequestID = requestID
	platformweb.WriteJSON(w, http.StatusOK, response)
}

func handleArchiveProductCatalogItem(auth authService, catalog productCatalogService, w http.ResponseWriter, r *http.Request) {
	requestID := platformweb.RequestIDFromContext(r.Context())
	state, ok := requireOrgWriter(auth, w, r)
	if !ok {
		return
	}
	if catalog == nil {
		platformweb.WriteError(w, http.StatusServiceUnavailable, requestID, "SERVICE_UNAVAILABLE", "Product catalog service unavailable")
		return
	}

	itemID, ok := parseProductCatalogItemID(w, r, requestID)
	if !ok {
		return
	}
	if err := catalog.Archive(r.Context(), state.Organization.ID, itemID); err != nil {
		writeProductCatalogError(w, requestID, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func productCatalogInput(request productCatalogRequest) moduleproductcatalog.Input {
	return moduleproductcatalog.Input{
		Name:        request.Name,
		SKU:         request.SKU,
		Description: request.Description,
		ItemType:    request.ItemType,
		UnitPrice:   request.UnitPrice,
		Currency:    request.Currency,
		UnitName:    request.UnitName,
		IsActive:    request.IsActive,
	}
}

func parseProductCatalogItemID(w http.ResponseWriter, r *http.Request, requestID string) (int64, bool) {
	itemID, err := strconv.ParseInt(r.PathValue("itemID"), 10, 64)
	if err != nil || itemID <= 0 {
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Invalid product catalog item ID")
		return 0, false
	}
	return itemID, true
}

func writeProductCatalogError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, moduleproductcatalog.ErrInvalidInput):
		platformweb.WriteError(w, http.StatusBadRequest, requestID, "BAD_REQUEST", "Provide a name, product or service type, non-negative price, three-letter currency, and unit")
	case errors.Is(err, moduleproductcatalog.ErrDuplicateSKU):
		platformweb.WriteError(w, http.StatusConflict, requestID, "CONFLICT", "A catalog item with that SKU already exists")
	case errors.Is(err, moduleproductcatalog.ErrNotFound):
		platformweb.WriteNotFound(w, requestID)
	default:
		platformweb.WriteError(w, http.StatusInternalServerError, requestID, "INTERNAL_SERVER_ERROR", "Unable to save product catalog item")
	}
}
