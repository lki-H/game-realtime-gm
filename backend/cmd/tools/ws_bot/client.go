package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type account struct {
	Index    int
	ID       int64
	Username string
	Token    string
}

type apiEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type loginData struct {
	Token  string `json:"token"`
	Player struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
	} `json:"player"`
}

type clientMessage struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	Data      any    `json:"data,omitempty"`
}

type serverMessage struct {
	Type      string          `json:"type"`
	RequestID string          `json:"request_id"`
	Code      int             `json:"code"`
	Message   string          `json:"message"`
	Data      json.RawMessage `json:"data"`
}

type squadData struct {
	Squad *struct {
		ID string `json:"id"`
	} `json:"squad"`
}

type botClient struct {
	account        account
	connection     *websocket.Conn
	requestTimeout time.Duration
	requestCounter atomic.Uint64
	requestMu      sync.Mutex
	measurements   *metrics
}

func ensureAccount(
	ctx context.Context,
	httpClient *http.Client,
	config runConfig,
	index int,
	measurements *metrics,
) (account, error) {
	username := fmt.Sprintf("%s_%03d", config.UsernamePrefix, index+1)
	if config.EnsureAccounts {
		startedAt := time.Now()
		err := registerAccount(ctx, httpClient, config.HTTPBaseURL, username, config.Password, index)
		if err != nil {
			measurements.RecordFailure("register", err)
			return account{}, err
		}
		measurements.RecordSuccess("register", time.Since(startedAt))
	}

	startedAt := time.Now()
	preparedAccount, err := loginAccount(ctx, httpClient, config.HTTPBaseURL, username, config.Password, index)
	if err != nil {
		measurements.RecordFailure("login", err)
		return account{}, err
	}
	measurements.RecordSuccess("login", time.Since(startedAt))
	preparedAccount.Index = index
	return preparedAccount, nil
}

func registerAccount(
	ctx context.Context,
	httpClient *http.Client,
	baseURL string,
	username string,
	password string,
	index int,
) error {
	status, envelope, err := postJSON(
		ctx,
		httpClient,
		strings.TrimRight(baseURL, "/")+"/api/register",
		fmt.Sprintf("day34-register-%03d", index+1),
		map[string]any{
			"username": username,
			"password": password,
			"nickname": fmt.Sprintf("Day34 Bot %03d", index+1),
		},
	)
	if err != nil {
		return withCategory("register_http", err)
	}
	if status == http.StatusCreated && envelope.Code == 0 {
		return nil
	}
	if status == http.StatusConflict && envelope.Code == 40901 {
		return nil
	}
	return withCategory(
		fmt.Sprintf("register_code_%d", envelope.Code),
		fmt.Errorf("register failed with HTTP %d and code %d", status, envelope.Code),
	)
}

func loginAccount(
	ctx context.Context,
	httpClient *http.Client,
	baseURL string,
	username string,
	password string,
	index int,
) (account, error) {
	status, envelope, err := postJSON(
		ctx,
		httpClient,
		strings.TrimRight(baseURL, "/")+"/api/login",
		fmt.Sprintf("day34-login-%03d", index+1),
		map[string]any{"username": username, "password": password},
	)
	if err != nil {
		return account{}, withCategory("login_http", err)
	}
	if status != http.StatusOK || envelope.Code != 0 {
		return account{}, withCategory(
			fmt.Sprintf("login_code_%d", envelope.Code),
			fmt.Errorf("login failed with HTTP %d and code %d", status, envelope.Code),
		)
	}

	var data loginData
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		return account{}, withCategory("login_decode", err)
	}
	if strings.TrimSpace(data.Token) == "" || data.Player.ID <= 0 {
		return account{}, withCategory("login_data", fmt.Errorf("login response missing token or player id"))
	}
	return account{ID: data.Player.ID, Username: data.Player.Username, Token: data.Token}, nil
}

func postJSON(
	ctx context.Context,
	httpClient *http.Client,
	endpoint string,
	requestID string,
	payload any,
) (int, apiEnvelope, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, apiEnvelope{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, apiEnvelope{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Request-ID", requestID)

	response, err := httpClient.Do(request)
	if err != nil {
		return 0, apiEnvelope{}, err
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return response.StatusCode, apiEnvelope{}, err
	}
	var envelope apiEnvelope
	if err := json.Unmarshal(responseBody, &envelope); err != nil {
		return response.StatusCode, apiEnvelope{}, err
	}
	return response.StatusCode, envelope, nil
}

func connectBot(
	ctx context.Context,
	config runConfig,
	preparedAccount account,
	measurements *metrics,
) (*botClient, error) {
	parsedURL, err := url.Parse(config.WebSocketURL)
	if err != nil {
		return nil, withCategory("ws_url", err)
	}
	query := parsedURL.Query()
	query.Set("token", preparedAccount.Token)
	parsedURL.RawQuery = query.Encode()

	dialer := *websocket.DefaultDialer
	dialer.HandshakeTimeout = config.RequestTimeout
	startedAt := time.Now()
	connection, response, err := dialer.DialContext(ctx, parsedURL.String(), nil)
	if response != nil && response.Body != nil {
		response.Body.Close()
	}
	if err != nil {
		measurements.RecordFailure("ws_connect", withCategory("ws_dial", err))
		return nil, withCategory("ws_dial", err)
	}

	if err := connection.SetReadDeadline(time.Now().Add(config.RequestTimeout)); err != nil {
		connection.Close()
		measurements.RecordFailure("ws_connect", withCategory("welcome_deadline", err))
		return nil, withCategory("welcome_deadline", err)
	}
	var welcome serverMessage
	if err := connection.ReadJSON(&welcome); err != nil {
		connection.Close()
		measurements.RecordFailure("ws_connect", withCategory("welcome_read", err))
		return nil, withCategory("welcome_read", err)
	}
	if err := connection.SetReadDeadline(time.Time{}); err != nil {
		connection.Close()
		measurements.RecordFailure("ws_connect", withCategory("welcome_deadline", err))
		return nil, withCategory("welcome_deadline", err)
	}
	measurements.RecordMessage(welcome.Type)
	if welcome.Type != "server.welcome" || welcome.Code != 0 {
		connection.Close()
		err := withCategory("welcome_message", fmt.Errorf("unexpected welcome type=%s code=%d", welcome.Type, welcome.Code))
		measurements.RecordFailure("ws_connect", err)
		return nil, err
	}

	measurements.RecordSuccess("ws_connect", time.Since(startedAt))
	return &botClient{
		account:        preparedAccount,
		connection:     connection,
		requestTimeout: config.RequestTimeout,
		measurements:   measurements,
	}, nil
}

func (client *botClient) Request(
	ctx context.Context,
	messageType string,
	data any,
) (serverMessage, time.Duration, error) {
	client.requestMu.Lock()
	defer client.requestMu.Unlock()

	requestID := fmt.Sprintf("day34_%03d_%06d", client.account.Index+1, client.requestCounter.Add(1))
	message := clientMessage{Type: messageType, RequestID: requestID, Data: data}
	startedAt := time.Now()

	if deadline, exists := ctx.Deadline(); exists {
		if err := client.connection.SetWriteDeadline(deadline); err != nil {
			return serverMessage{}, 0, withCategory("ws_write_deadline", err)
		}
	} else if err := client.connection.SetWriteDeadline(time.Now().Add(client.requestTimeout)); err != nil {
		return serverMessage{}, 0, withCategory("ws_write_deadline", err)
	}
	if err := client.connection.WriteJSON(message); err != nil {
		return serverMessage{}, time.Since(startedAt), withCategory("ws_write", err)
	}

	readDeadline := time.Now().Add(client.requestTimeout)
	if deadline, exists := ctx.Deadline(); exists && deadline.Before(readDeadline) {
		readDeadline = deadline
	}
	if err := client.connection.SetReadDeadline(readDeadline); err != nil {
		return serverMessage{}, time.Since(startedAt), withCategory("ws_read_deadline", err)
	}
	defer client.connection.SetReadDeadline(time.Time{})

	for {
		var response serverMessage
		if err := client.connection.ReadJSON(&response); err != nil {
			return serverMessage{}, time.Since(startedAt), withCategory("ws_read", err)
		}
		client.measurements.RecordMessage(response.Type)
		if response.RequestID != requestID {
			continue
		}
		duration := time.Since(startedAt)
		if response.Type == "server.error" || response.Code != 0 {
			return response, duration, withCategory(
				fmt.Sprintf("server_code_%d", response.Code),
				fmt.Errorf("server returned type=%s code=%d message=%s", response.Type, response.Code, response.Message),
			)
		}
		return response, duration, nil
	}
}

func (client *botClient) Close() {
	client.requestMu.Lock()
	defer client.requestMu.Unlock()
	deadline := time.Now().Add(time.Second)
	_ = client.connection.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "day34 complete"), deadline)
	_ = client.connection.Close()
}
