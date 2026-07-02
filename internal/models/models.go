package models

import "time"

type SearchResult struct {
	Rank            int    `json:"rank"`
	URL             string `json:"url"`
	Domain          string `json:"domain"`
	Title           string `json:"title"`
	Snippet         string `json:"snippet"`
	FaviconURL      string `json:"faviconUrl"`
	IsWhitelisted   bool   `json:"isWhitelisted"`
	WhitelistWeight int    `json:"whitelistWeight"`
}

type SearchRecord struct {
	ID              string
	QueryText       string
	NormalizedQuery string
	QueryHash       string
	ServedFromCache bool
	Results         []SearchResult
	Summary         string
	Model           string
	CreatedAt       time.Time
}
