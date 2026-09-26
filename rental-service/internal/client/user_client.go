package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

var ErrAccountInactive = errors.New("account is inactive or token is invalid")

type UserClient struct {
	baseURL, apiKey string
	http            *http.Client
}

func NewUserClient(baseURL, apiKey string) *UserClient {
	return &UserClient{baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, http: &http.Client{Timeout: 5 * time.Second}}
}

// VerifyCaller confirms a state-changing caller still has an active account.
func (u *UserClient) VerifyCaller(token string) error {
	body, err := json.Marshal(map[string]string{"token": token})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, u.baseURL+"/auth/verify", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Key", u.apiKey)
	res, err := u.http.Do(req)
	if err != nil {
		return ErrDependency
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return ErrAccountInactive
	}
	if res.StatusCode >= 400 {
		return ErrDependency
	}
	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Active bool `json:"active"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil || !payload.Success {
		return ErrDependency
	}
	if !payload.Data.Active {
		return ErrAccountInactive
	}
	return nil
}
