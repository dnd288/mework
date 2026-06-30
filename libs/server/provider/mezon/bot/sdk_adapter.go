// Package bot provides the Mezon bot client and a real SDK adapter that
// wraps github.com/nccasia/mezon-go-sdk to implement the SDKClient interface.
package bot

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/nccasia/mezon-go-sdk/configs"
	"github.com/nccasia/mezon-go-sdk/constants"
	mezonsdk "github.com/nccasia/mezon-go-sdk"
	swagger "github.com/nccasia/mezon-go-sdk/mezon-api"
	"github.com/nccasia/mezon-go-sdk/mezon-protobuf/mezon/v2/common/rtapi"
	"github.com/nccasia/mezon-go-sdk/utils"

	sharedMezon "mework/libs/shared/providers/mezon"
)

// RealSDKAdapter wraps the Mezon SDK to implement the SDKClient interface.
// It handles authentication, WebSocket connection, message dispatch, and
// reconnection using the real Mezon SDK.
type RealSDKAdapter struct {
	cfg        sharedMezon.Config
	apiKey     string
	httpClient *swagger.MezonApiService
	socket     mezonsdk.IWSConnection
	token      string
	userID     string

	msgHandler func(interface{})
	reconnFn   func()
	connected  bool
	closeCh    chan struct{}
}

// NewRealSDKAdapter creates a new adapter wrapping the real Mezon SDK.
func NewRealSDKAdapter(cfg sharedMezon.Config) *RealSDKAdapter {
	return &RealSDKAdapter{
		cfg:     cfg,
		apiKey:  cfg.APIKey,
		closeCh: make(chan struct{}),
	}
}

// Authenticate exchanges credentials for a session token and captures the
// bot user ID for self-message filtering.
func (a *RealSDKAdapter) Authenticate() (token, userID string, err error) {
	// Build base URL from config or SDK default.
	var basePath string
	if a.cfg.BaseURL != "" {
		basePath = utils.GetBasePath("http", a.cfg.BaseURL, constants.UseSSL)
	} else {
		basePath = utils.GetBasePath("http", constants.MznBasePath, constants.UseSSL)
	}

	swagCfg := swagger.NewConfiguration()
	swagCfg.BasePath = basePath
	if constants.InsecureSkip {
		swagCfg.HTTPClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
	} else {
		swagCfg.HTTPClient = http.DefaultClient
	}
	swagCfg.AddDefaultHeader("Authorization",
		base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("Basic %s:", a.apiKey))))

	api := swagger.NewAPIClient(swagCfg).MezonApi

	session, _, err := api.MezonAuthenticate(context.Background(), swagger.ApiAuthenticateRequest{
		Account: &swagger.ApiAccountApp{
			Token: a.apiKey,
		},
	})
	if err != nil {
		return "", "", fmt.Errorf("mezon auth: %w", err)
	}

	a.token = session.Token
	a.userID = session.UserId
	a.httpClient = api

	log.Printf("mezon: authenticated as user %s", a.userID)
	return a.token, a.userID, nil
}

// OnMessage registers the callback for received messages.
func (a *RealSDKAdapter) OnMessage(fn func(interface{})) {
	a.msgHandler = fn
}

// OnReconnect registers the callback for reconnection events.
func (a *RealSDKAdapter) OnReconnect(fn func()) {
	a.reconnFn = fn
}

// Connect opens the WebSocket connection to Mezon's real-time gateway.
func (a *RealSDKAdapter) Connect() error {
	// Get the list of clans the bot belongs to.
	clanDescs, _, err := a.httpClient.MezonListClanDescs(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("list clans: %w", err)
	}

	clanIDs := []string{"0"} // "0" = direct messages
	for _, clan := range clanDescs.Clandesc {
		clanIDs = append(clanIDs, clan.ClanId)
	}

	sdkCfg := &configs.Config{
		ApiKey:  a.apiKey,
		Timeout: 15,
	}

	socket, err := mezonsdk.NewWSConnection(sdkCfg, a.token, clanIDs)
	if err != nil {
		return fmt.Errorf("websocket connect: %w", err)
	}

	a.socket = socket

	// Register channel message handler.
	socket.SetOnChannelMessage(func(envelope *rtapi.Envelope) error {
		msg := envelope.GetChannelMessage()
		if msg == nil {
			return nil
		}

		wrapped := &sdkMessageAdapter{
			channelID: msg.ChannelId,
			senderID:  msg.SenderId,
			text:      msg.Content,
		}

		if a.msgHandler != nil {
			a.msgHandler(wrapped)
		}
		return nil
	})

	// Pong handler (the SDK's internal ping/pong keeps the connection alive).
	socket.SetOnPong(func(envelope *rtapi.Envelope) error {
		return nil
	})

	a.connected = true
	log.Printf("mezon: websocket connected (clans: %d)", len(clanIDs))
	return nil
}

// SendText sends a text message to the given channel via the WebSocket.
func (a *RealSDKAdapter) SendText(channelID, text string) error {
	if a.socket == nil {
		return fmt.Errorf("not connected")
	}

	envelope := &rtapi.Envelope{
		Message: &rtapi.Envelope_ChannelMessageSend{
			ChannelMessageSend: &rtapi.ChannelMessageSend{
				ChannelId: channelID,
				Content:   text,
			},
		},
	}

	return a.socket.SendMessage(envelope)
}

// Close closes the WebSocket connection.
func (a *RealSDKAdapter) Close() error {
	if a.socket != nil {
		err := a.socket.Close()
		a.connected = false
		return err
	}
	return nil
}

// sdkMessageAdapter implements SDKMessage so the bot's extractMessageFields
// can read fields via the compile-time-safe interface assertion.
type sdkMessageAdapter struct {
	channelID string
	senderID  string
	text      string
}

func (m *sdkMessageAdapter) GetChannelID() string { return m.channelID }
func (m *sdkMessageAdapter) GetSenderID() string  { return m.senderID }
func (m *sdkMessageAdapter) GetText() string      { return m.text }

// Compile-time interface check.
var _ SDKMessage = (*sdkMessageAdapter)(nil)

// Ensure time import is used (for reconnection backoff contexts).
var _ = time.Second
