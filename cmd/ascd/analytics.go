package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	blitzAgentSessionEnv = "BLITZ_AGENT_SESSION"
	endpointEnv          = "BLITZ_ANALYTICS_ENDPOINT"
	tokenEnv             = "BLITZ_ANALYTICS_TOKEN"
	deviceIDEnv          = "BLITZ_ANALYTICS_DEVICE_ID"
	appVersionEnv        = "BLITZ_ANALYTICS_APP_VERSION"
	osVersionEnv         = "BLITZ_ANALYTICS_OS_VERSION"
	analyticsPayloadEnv  = "BLITZ_ANALYTICS_PAYLOAD"
	analyticsUploadArg   = "__blitz_analytics_upload__"
)

type analyticsPayload struct {
	EventID     string `json:"event_id"`
	ClientAt    string `json:"client_at"`
	DeviceID    string `json:"device_id"`
	AppVersion  string `json:"app_version"`
	OSVersion   string `json:"os_version"`
	EventName   string `json:"event_name"`
	Source      string `json:"source,omitempty"`
	CommandType string `json:"command_type,omitempty"`
	Success     *bool  `json:"success,omitempty"`
	DurationMS  *int64 `json:"duration_ms,omitempty"`
}

func enqueueAgentDirectAnalytics(commandType string, success bool, startedAt time.Time) {
	if strings.TrimSpace(os.Getenv(blitzAgentSessionEnv)) != "1" {
		return
	}

	endpoint := strings.TrimSpace(os.Getenv(endpointEnv))
	token := strings.TrimSpace(os.Getenv(tokenEnv))
	deviceID := strings.TrimSpace(os.Getenv(deviceIDEnv))
	appVersion := strings.TrimSpace(os.Getenv(appVersionEnv))
	osVersion := strings.TrimSpace(os.Getenv(osVersionEnv))
	if endpoint == "" || token == "" || deviceID == "" || appVersion == "" || osVersion == "" {
		return
	}

	durationMS := time.Since(startedAt).Milliseconds()
	if durationMS < 0 {
		durationMS = 0
	}

	payload := analyticsPayload{
		EventID:     randomEventID(),
		ClientAt:    time.Now().UTC().Format(time.RFC3339Nano),
		DeviceID:    deviceID,
		AppVersion:  appVersion,
		OSVersion:   osVersion,
		EventName:   "asc_usage",
		Source:      "agent_direct",
		CommandType: strings.TrimSpace(commandType),
		Success:     &success,
		DurationMS:  &durationMS,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return
	}

	executable, err := os.Executable()
	if err != nil {
		return
	}

	command := exec.Command(executable, analyticsUploadArg)
	command.Env = []string{
		analyticsPayloadEnv + "=" + base64.StdEncoding.EncodeToString(body),
		endpointEnv + "=" + endpoint,
		tokenEnv + "=" + token,
	}
	if err := command.Start(); err != nil {
		return
	}
	_ = command.Process.Release()
}

func runAnalyticsUploadSubprocess() int {
	encodedPayload := strings.TrimSpace(os.Getenv(analyticsPayloadEnv))
	if encodedPayload == "" {
		return 0
	}

	body, err := base64.StdEncoding.DecodeString(encodedPayload)
	if err != nil {
		return 0
	}

	endpoint := strings.TrimSpace(os.Getenv(endpointEnv))
	token := strings.TrimSpace(os.Getenv(tokenEnv))
	if endpoint == "" || token == "" {
		return 0
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	return 0
}

func randomEventID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return time.Now().UTC().Format("20060102150405.000000000")
	}

	var out [36]byte
	hex.Encode(out[0:8], raw[0:4])
	out[8] = '-'
	hex.Encode(out[9:13], raw[4:6])
	out[13] = '-'
	hex.Encode(out[14:18], raw[6:8])
	out[18] = '-'
	hex.Encode(out[19:23], raw[8:10])
	out[23] = '-'
	hex.Encode(out[24:36], raw[10:16])
	return string(out[:])
}
