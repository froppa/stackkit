package main

import (
	"net/http"
)

// ListPets handles GET /pets
// List all pets
func ListPets(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Hello from ListPets"))
}
