package proxmox

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
)

// Client is an HTTP client for the Proxmox VE REST API.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient creates a new Proxmox API client.
// token format: "PVEAPIToken=user@realm!tokenid=secret" or just "user@realm!tokenid=secret".
func NewClient(host string, port int, token string, insecure bool) *Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure},
	}
	return &Client{
		baseURL:    fmt.Sprintf("https://%s:%d/api2/json", host, port),
		token:      ensurePrefix(token),
		httpClient: &http.Client{Transport: transport},
	}
}

func ensurePrefix(token string) string {
	if len(token) > 15 && token[:15] == "PVEAPIToken=" {
		return token
	}
	return "PVEAPIToken=" + token
}

// get performs a GET request and decodes the "data" field into result.
func (c *Client) get(path string, result interface{}) error {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GET %s: HTTP %d: %s", path, resp.StatusCode, body)
	}

	return decodeData(resp.Body, result)
}

// post performs a POST request with form-encoded body and returns the raw "data" value.
func (c *Client) post(path string, params url.Values) (string, error) {
	encoded := params.Encode()
	if os.Getenv("SIMPLECD_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "[DEBUG] POST %s\n[DEBUG] body: %s\n[DEBUG] auth header: %s\n", c.baseURL+path, encoded, maskToken(c.token))
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewBufferString(encoded))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", c.token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("POST %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if os.Getenv("SIMPLECD_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "[DEBUG] response: HTTP %d: %s\n", resp.StatusCode, body)
	}
	if resp.StatusCode >= 400 {
		// Try to extract Proxmox error details from the response
		var errBody struct {
			Errors  map[string]string `json:"errors"`
			Message string            `json:"message"`
		}
		if json.Unmarshal(body, &errBody) == nil {
			if len(errBody.Errors) > 0 {
				for field, msg := range errBody.Errors {
					return "", fmt.Errorf("POST %s: HTTP %d: field %q: %s", path, resp.StatusCode, field, msg)
				}
			}
			if errBody.Message != "" {
				return "", fmt.Errorf("POST %s: HTTP %d: %s", path, resp.StatusCode, errBody.Message)
			}
		}
		return "", fmt.Errorf("POST %s: HTTP %d: %s", path, resp.StatusCode, body)
	}

	// The data field is typically the UPID string for async operations
	var envelope struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}
	return envelope.Data, nil
}

// Ping tests connectivity by fetching the API version endpoint.
func (c *Client) Ping() error {
	var result interface{}
	return c.get("/version", &result)
}

func maskToken(t string) string {
	// Show prefix up to "=" after tokenid, then mask the secret
	if i := len("PVEAPIToken="); len(t) > i {
		rest := t[i:]
		if j := len(rest) - 8; j > 0 {
			return t[:i] + rest[:j] + "****"
		}
	}
	return "****"
}

func decodeData(r io.Reader, result interface{}) error {
	body, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	// Proxmox wraps everything in {"data": ...}
	var raw struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return fmt.Errorf("parsing envelope: %w", err)
	}
	if result == nil {
		return nil
	}
	return json.Unmarshal(raw.Data, result)
}
