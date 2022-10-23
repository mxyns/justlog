package helix

import (
	"net/http"
	"strings"
	"sync"
	"time"

	helixClient "github.com/nicklaw5/helix"
	log "github.com/sirupsen/logrus"
)

// Client wrapper for helix
type Client struct {
	clientID       string
	clientSecret   string
	appAccessToken string
	HelixClient    *helixClient.Client
	httpClient     *http.Client
}

var (
	userCacheByID       sync.Map
	userCacheByUsername sync.Map
)

type TwitchApiClient interface {
	GetUsersByUserIds([]string) (map[string]UserData, error)
	GetUsersByUsernames([]string) (map[string]UserData, error)
	GetChannelInformationByChannelIds([]string) (map[string]ChannelData, error)
}

// NewClient Create helix Client
func NewClient(clientID string, clientSecret string) Client {
	client, err := helixClient.NewClient(&helixClient.Options{
		ClientID:     clientID,
		ClientSecret: clientSecret,
	})
	if err != nil {
		panic(err)
	}

	resp, err := client.RequestAppAccessToken([]string{})
	if err != nil {
		panic(err)
	}
	log.Infof("Requested access token, response: %d, expires in: %d", resp.StatusCode, resp.Data.ExpiresIn)
	client.SetAppAccessToken(resp.Data.AccessToken)

	return Client{
		clientID:       clientID,
		clientSecret:   clientSecret,
		appAccessToken: resp.Data.AccessToken,
		HelixClient:    client,
		httpClient:     &http.Client{},
	}
}

// UserData exported data from twitch
type UserData struct {
	ID              string `json:"id"`
	Login           string `json:"login"`
	DisplayName     string `json:"display_name"`
	Type            string `json:"type"`
	BroadcasterType string `json:"broadcaster_type"`
	Description     string `json:"description"`
	ProfileImageURL string `json:"profile_image_url"`
	OfflineImageURL string `json:"offline_image_url"`
	ViewCount       int    `json:"view_count"`
	Email           string `json:"email"`
}

type StreamStatus bool

const (
	StreamStatusUnknown = false
	StreamStatusKnown   = true
)

// ChannelData exported data from twitch
type ChannelData struct {
	BroadcasterID       string           `json:"broadcaster_id"`
	BroadcasterName     string           `json:"broadcaster_name"`
	GameName            string           `json:"game_name"`
	GameID              string           `json:"game_id"`
	BroadcasterLanguage string           `json:"broadcaster_language"`
	Title               string           `json:"title"`
	Delay               int              `json:"integer"`
	StreamStatus        StreamStatus     `json:"stream_status"`
	IsLive              bool             `json:"is_live"`
	StartedAt           helixClient.Time `json:"started_at"`
}

// StartRefreshTokenRoutine refresh our token
func (c *Client) StartRefreshTokenRoutine() {
	ticker := time.NewTicker(24 * time.Hour)

	for range ticker.C {
		resp, err := c.HelixClient.RequestAppAccessToken([]string{})
		if err != nil {
			log.Error(err)
			continue
		}
		log.Infof("Requested access token from routine, response: %d, expires in: %d", resp.StatusCode, resp.Data.ExpiresIn)

		c.HelixClient.SetAppAccessToken(resp.Data.AccessToken)
	}
}

func chunkBy(items []string, chunkSize int) (chunks [][]string) {
	for chunkSize < len(items) {
		items, chunks = items[chunkSize:], append(chunks, items[0:chunkSize:chunkSize])
	}

	return append(chunks, items)
}

// GetUsersByUserIds receive userData for given ids
func (c *Client) GetUsersByUserIds(userIDs []string) (map[string]UserData, error) {
	var filteredUserIDs []string

	for _, id := range userIDs {
		if _, ok := userCacheByID.Load(id); !ok {
			filteredUserIDs = append(filteredUserIDs, id)
		}
	}

	if len(filteredUserIDs) > 0 {
		chunks := chunkBy(filteredUserIDs, 100)

		for _, chunk := range chunks {
			resp, err := c.HelixClient.GetUsers(&helixClient.UsersParams{
				IDs: chunk,
			})
			if err != nil {
				return map[string]UserData{}, err
			}

			log.Infof("%d GetUsersByUserIds %v", resp.StatusCode, chunk)

			for _, user := range resp.Data.Users {
				data := &UserData{
					ID:              user.ID,
					Login:           user.Login,
					DisplayName:     user.Login,
					Type:            user.Type,
					BroadcasterType: user.BroadcasterType,
					Description:     user.Description,
					ProfileImageURL: user.ProfileImageURL,
					OfflineImageURL: user.OfflineImageURL,
					ViewCount:       user.ViewCount,
					Email:           user.Email,
				}
				userCacheByID.Store(user.ID, data)
				userCacheByUsername.Store(user.Login, data)
			}
		}
	}

	result := make(map[string]UserData)

	for _, id := range userIDs {
		value, ok := userCacheByID.Load(id)
		if !ok {
			log.Warningf("Could not find userId, channel might be banned: %s", id)
			continue
		}
		result[id] = *(value.(*UserData))
	}

	return result, nil
}

// GetUsersByUsernames fetches userdata from helix
func (c *Client) GetUsersByUsernames(usernames []string) (map[string]UserData, error) {
	var filteredUsernames []string

	for _, username := range usernames {
		username = strings.ToLower(username)
		if _, ok := userCacheByUsername.Load(username); !ok {
			filteredUsernames = append(filteredUsernames, username)
		}
	}

	if len(filteredUsernames) > 0 {
		chunks := chunkBy(filteredUsernames, 100)

		for _, chunk := range chunks {
			resp, err := c.HelixClient.GetUsers(&helixClient.UsersParams{
				Logins: chunk,
			})
			if err != nil {
				return map[string]UserData{}, err
			}

			log.Infof("%d GetUsersByUsernames %v", resp.StatusCode, chunk)

			for _, user := range resp.Data.Users {
				data := &UserData{
					ID:              user.ID,
					Login:           user.Login,
					DisplayName:     user.Login,
					Type:            user.Type,
					BroadcasterType: user.BroadcasterType,
					Description:     user.Description,
					ProfileImageURL: user.ProfileImageURL,
					OfflineImageURL: user.OfflineImageURL,
					ViewCount:       user.ViewCount,
					Email:           user.Email,
				}
				userCacheByID.Store(user.ID, data)
				userCacheByUsername.Store(user.Login, data)
			}
		}
	}

	result := make(map[string]UserData)

	for _, username := range usernames {
		username = strings.ToLower(username)
		value, ok := userCacheByUsername.Load(username)
		if !ok {
			log.Warningf("Could not find username, channel might be banned: %s", username)
			continue
		}
		result[username] = *(value.(*UserData))
	}

	return result, nil
}

// GetChannelInformationByChannelIds receive userData for given ids
func (c *Client) GetChannelInformationByChannelIds(channelIds []string) (map[string]ChannelData, error) {

	infoResp, err := c.HelixClient.GetChannelInformation(&helixClient.GetChannelInformationParams{
		BroadcasterIDs: channelIds,
	})

	if err != nil {
		return map[string]ChannelData{}, err
	}

	result := make(map[string]ChannelData)

	for _, channel := range infoResp.Data.Channels {
		data := ChannelData{
			BroadcasterID:       channel.BroadcasterID,
			BroadcasterName:     channel.BroadcasterName,
			GameName:            channel.GameName,
			GameID:              channel.GameID,
			BroadcasterLanguage: channel.BroadcasterLanguage,
			Title:               channel.Title,
			Delay:               channel.Delay,
			StreamStatus:        StreamStatusUnknown,
			IsLive:              false,
		}

		if searchResp, err := c.HelixClient.SearchChannels(&helixClient.SearchChannelsParams{
			Channel:  data.BroadcasterName,
			After:    "",
			First:    20,
			LiveOnly: true,
		}); err == nil {
			for _, searchResult := range searchResp.Data.Channels {
				if searchResult.ID != data.BroadcasterID || !searchResult.IsLive {
					continue
				}
				data.StreamStatus = StreamStatusKnown
				data.IsLive = searchResult.IsLive
				data.StartedAt = searchResult.StartedAt
				break
			}
		}

		result[channel.BroadcasterID] = data
	}

	return result, nil
}
