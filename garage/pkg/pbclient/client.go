// Package pbclient provides a client for PocketBase-HA API
// Used by tiered storage for multi-device file tracking
package pbclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is a PocketBase API client
type Client struct {
	BaseURL    string
	Token      string
	DeviceName string
	httpClient *http.Client
}

// GarageFile represents a file in the garage_files collection
type GarageFile struct {
	ID             string `json:"id,omitempty"`
	Path           string `json:"path"`
	Filename       string `json:"filename"`
	Size           int64  `json:"size"`
	Hash           string `json:"hash"`
	MimeType       string `json:"mime_type"`
	Tier           int    `json:"tier"`
	R2Key          string `json:"r2_key"`
	B2Key          string `json:"b2_key"`
	CurrentVersion int    `json:"current_version"`
	IsDeleted      bool   `json:"is_deleted"`
	Created        string `json:"created,omitempty"`
	Updated        string `json:"updated,omitempty"`
}

// FileVersion represents a version in the file_versions collection
type FileVersion struct {
	ID         string `json:"id,omitempty"`
	FilePath   string `json:"file_path"`
	VersionNum int    `json:"version_num"`
	DeviceName string `json:"device_name"`
	Size       int64  `json:"size"`
	Hash       string `json:"hash"`
	R2Key      string `json:"r2_key"`
	IsConflict bool   `json:"is_conflict"`
	Created    string `json:"created,omitempty"`
}

// DeviceCache represents a device's local cache entry
type DeviceCache struct {
	ID         string `json:"id,omitempty"`
	DeviceName string `json:"device_name"`
	FilePath   string `json:"file_path"`
	VersionNum int    `json:"version_num"`
	LocalPath  string `json:"local_path"`
	IsDirty    bool   `json:"is_dirty"`
	LastSynced string `json:"last_synced"`
}

// NewClient creates a new PocketBase client
func NewClient(baseURL, deviceName string) *Client {
	return &Client{
		BaseURL:    baseURL,
		DeviceName: deviceName,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Authenticate logs in as admin and stores the token
func (c *Client) Authenticate(email, password string) error {
	payload := map[string]string{
		"identity": email,
		"password": password,
	}
	body, _ := json.Marshal(payload)

	resp, err := c.httpClient.Post(
		c.BaseURL+"/api/collections/_superusers/auth-with-password",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("auth request failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("auth decode failed: %w", err)
	}
	if result.Token == "" {
		return fmt.Errorf("auth failed: no token returned")
	}

	c.Token = result.Token
	return nil
}

// request makes an authenticated HTTP request
func (c *Client) request(method, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.BaseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", c.Token)
	}

	return c.httpClient.Do(req)
}

// GetFile retrieves file metadata by path
func (c *Client) GetFile(path string) (*GarageFile, error) {
	resp, err := c.request("GET", "/api/collections/garage_files/records?filter=path='"+path+"'", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Items []GarageFile `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if len(result.Items) == 0 {
		return nil, nil
	}
	return &result.Items[0], nil
}

// CreateFile creates a new file record
func (c *Client) CreateFile(file *GarageFile) error {
	resp, err := c.request("POST", "/api/collections/garage_files/records", file)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create file failed: %s", body)
	}

	return json.NewDecoder(resp.Body).Decode(file)
}

// UpdateFile updates an existing file record
func (c *Client) UpdateFile(file *GarageFile) error {
	resp, err := c.request("PATCH", "/api/collections/garage_files/records/"+file.ID, file)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update file failed: %s", body)
	}

	return nil
}

// CreateVersion creates a new file version record
func (c *Client) CreateVersion(version *FileVersion) error {
	version.DeviceName = c.DeviceName
	resp, err := c.request("POST", "/api/collections/file_versions/records", version)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create version failed: %s", body)
	}

	return json.NewDecoder(resp.Body).Decode(version)
}

// UpdateDeviceCache updates this device's cache entry for a file
func (c *Client) UpdateDeviceCache(filePath string, versionNum int, localPath string, isDirty bool) error {
	// Check if entry exists
	resp, err := c.request("GET", fmt.Sprintf("/api/collections/device_cache/records?filter=device_name='%s' AND file_path='%s'", c.DeviceName, filePath), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Items []DeviceCache `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	cache := DeviceCache{
		DeviceName: c.DeviceName,
		FilePath:   filePath,
		VersionNum: versionNum,
		LocalPath:  localPath,
		IsDirty:    isDirty,
		LastSynced: time.Now().UTC().Format(time.RFC3339),
	}

	if len(result.Items) > 0 {
		// Update existing
		cache.ID = result.Items[0].ID
		resp, err = c.request("PATCH", "/api/collections/device_cache/records/"+cache.ID, cache)
	} else {
		// Create new
		resp, err = c.request("POST", "/api/collections/device_cache/records", cache)
	}
	if err != nil {
		return err
	}
	resp.Body.Close()

	return nil
}

// ListFilesNeedingSync returns files that are dirty (modified locally, not synced)
func (c *Client) ListFilesNeedingSync() ([]DeviceCache, error) {
	resp, err := c.request("GET", fmt.Sprintf("/api/collections/device_cache/records?filter=device_name='%s' AND is_dirty=true", c.DeviceName), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Items []DeviceCache `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Items, nil
}

// ListAllFiles returns all tracked files
func (c *Client) ListAllFiles() ([]GarageFile, error) {
	resp, err := c.request("GET", "/api/collections/garage_files/records?filter=is_deleted=false", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Items []GarageFile `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Items, nil
}

// GetFileStats returns counts for each tier
func (c *Client) GetFileStats() (local, r2, b2 int, err error) {
	files, err := c.ListAllFiles()
	if err != nil {
		return 0, 0, 0, err
	}

	for _, f := range files {
		switch f.Tier {
		case 0:
			local++
		case 1:
			r2++
		case 2:
			b2++
		}
	}

	return local, r2, b2, nil
}

// RegisterDevice registers this device if not already registered
func (c *Client) RegisterDevice(deviceType, platform string) error {
	// Check if exists
	resp, err := c.request("GET", fmt.Sprintf("/api/collections/devices/records?filter=device_name='%s'", c.DeviceName), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Items []struct{ ID string } `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	device := map[string]interface{}{
		"device_name": c.DeviceName,
		"device_type": deviceType,
		"platform":    platform,
		"last_seen":   time.Now().UTC().Format(time.RFC3339),
		"is_online":   true,
	}

	if len(result.Items) > 0 {
		// Update
		resp, err = c.request("PATCH", "/api/collections/devices/records/"+result.Items[0].ID, device)
	} else {
		// Create
		resp, err = c.request("POST", "/api/collections/devices/records", device)
	}
	if err != nil {
		return err
	}
	resp.Body.Close()

	return nil
}
