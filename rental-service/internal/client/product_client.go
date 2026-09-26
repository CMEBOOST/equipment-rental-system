package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrProductNotFound    = errors.New("product not found")
	ErrProductUnavailable = errors.New("product is unavailable")
	ErrDependency         = errors.New("dependent service unavailable")
)

type Product struct {
	ID          uuid.UUID `json:"id"`
	PricePerDay float64   `json:"price_per_day"`
	Status      string    `json:"status"`
}

type ProductClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func NewProductClient(baseURL, apiKey string) *ProductClient {
	return &ProductClient{baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, http: &http.Client{Timeout: 5 * time.Second}}
}

func (p *ProductClient) Get(id uuid.UUID) (*Product, error) {
	req, err := http.NewRequest(http.MethodGet, p.baseURL+"/products/"+id.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Internal-Key", p.apiKey)
	res, err := p.http.Do(req)
	if err != nil {
		return nil, ErrDependency
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return nil, ErrProductNotFound
	}
	if res.StatusCode >= 500 {
		return nil, ErrDependency
	}
	if res.StatusCode >= 400 {
		return nil, ErrDependency
	}
	var payload struct {
		Success bool    `json:"success"`
		Data    Product `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil || !payload.Success {
		return nil, ErrDependency
	}
	return &payload.Data, nil
}

func (p *ProductClient) SetStatus(id uuid.UUID, status string) error {
	body, err := json.Marshal(map[string]string{"status": status})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPatch, p.baseURL+"/products/"+id.String()+"/status", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Key", p.apiKey)
	res, err := p.http.Do(req)
	if err != nil {
		return ErrDependency
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return ErrProductNotFound
	}
	if res.StatusCode == http.StatusConflict {
		return ErrProductUnavailable
	}
	if res.StatusCode >= 400 {
		return ErrDependency
	}
	return nil
}
