package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListPets(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/foo", nil)
	w := httptest.NewRecorder()
	ListPets(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("Expected 200")
	}
}
