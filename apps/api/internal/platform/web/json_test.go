package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONBodyRejectsOversizedBody(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"too large"}`))
	var payload struct {
		Name string `json:"name"`
	}

	err := DecodeJSONBody(recorder, request, &payload, 4)
	if !errors.Is(err, ErrRequestBodyTooLarge) {
		t.Fatalf("expected oversized body error, got %v", err)
	}
}

func TestDecodeJSONBodyDecodesValidBody(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"demo"}`))
	var payload struct {
		Name string `json:"name"`
	}

	if err := DecodeJSONBody(recorder, request, &payload, 1024); err != nil {
		t.Fatalf("expected valid body to decode, got %v", err)
	}
	if payload.Name != "demo" {
		t.Fatalf("expected decoded name demo, got %q", payload.Name)
	}
}
