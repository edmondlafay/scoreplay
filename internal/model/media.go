package model

import "time"

type MediaType string

const (
	MediaTypePhoto MediaType = "photo"
	MediaTypeVideo MediaType = "video"
)

type Media struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Type      MediaType `json:"type"`
	FileURL   string    `json:"file_url"`
	Tags      []Tag     `json:"tags"`
	CreatedAt time.Time `json:"created_at"`
}
