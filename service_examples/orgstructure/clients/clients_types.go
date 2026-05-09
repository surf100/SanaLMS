package clients

import (
	"time"
)

type CreateClientRequest struct {
	Name      string  `json:"name"`
	Domain    *string `json:"domain"`
	Language  *string `json:"language"`
	UserLimit *int    `json:"user_limit"`
}

type Client struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Domain    *string   `json:"domain"`
	Language  *string   `json:"language"`
	UserLimit *int      `json:"user_limit"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

type GetClientResponse struct {
	Client Client `json:"client"`
}

type ListClientsResponse struct {
	Clients []Client `json:"clients"`
	Total   int      `json:"total"`
}
