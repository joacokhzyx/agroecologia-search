package cache

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Client habla con la REST API de Upstash Redis, así que no hace falta
// instalar ni correr Redis en ningún lado (ni local ni en el server).
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func New(baseURL, token string) *Client {
	return &Client{
		baseURL:    baseURL,
		token:      token,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

type upstashResponse struct {
	Result *string `json:"result"`
	Error  string  `json:"error"`
}

// Get devuelve el valor cacheado para key, y si se encontró o no.
func (c *Client) Get(key string) (string, bool, error) {
	endpoint := fmt.Sprintf("%s/get/%s", c.baseURL, url.PathEscape(key))
	body, err := c.do(endpoint)
	if err != nil {
		return "", false, err
	}
	if body.Result == nil {
		return "", false, nil
	}
	return *body.Result, true, nil
}

// Set guarda value bajo key con un TTL en segundos.
func (c *Client) Set(key, value string, ttlSeconds int) error {
	endpoint := fmt.Sprintf(
		"%s/set/%s/%s/EX/%s",
		c.baseURL,
		url.PathEscape(key),
		url.PathEscape(value),
		strconv.Itoa(ttlSeconds),
	)
	_, err := c.do(endpoint)
	return err
}

func (c *Client) do(endpoint string) (*upstashResponse, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var parsed upstashResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("cache: respuesta inesperada %q: %w", raw, err)
	}
	if parsed.Error != "" {
		return nil, fmt.Errorf("cache: error de upstash: %s", parsed.Error)
	}
	return &parsed, nil
}
