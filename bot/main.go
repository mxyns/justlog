package bot

import (
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/mxyns/justlog/config"
	"github.com/mxyns/justlog/filelog"

	twitch "github.com/gempir/go-twitch-irc/v3"
	"github.com/mxyns/justlog/helix"
	log "github.com/sirupsen/logrus"
)

// Bot basic logging bot
type Bot struct {
	startTime   time.Time
	cfg         *config.Config
	helixClient helix.TwitchApiClient
	logger      filelog.Logger
	worker      []*worker
	channels    map[string]helix.UserData
	clearchats  sync.Map
	OptoutCodes sync.Map
}

type worker struct {
	client         *twitch.Client
	joinedChannels []string
}

// NewBot create new bot instance
func NewBot(cfg *config.Config, helixClient helix.TwitchApiClient, fileLogger filelog.Logger) *Bot {
	channels, err := helixClient.GetUsersByUserIds(cfg.Channels)
	if err != nil {
		log.Fatalf("[bot] failed to load configured channels %s", err.Error())
	}

	return &Bot{
		cfg:         cfg,
		helixClient: helixClient,
		logger:      fileLogger,
		channels:    channels,
		worker:      []*worker{},
		OptoutCodes: sync.Map{},
	}
}

func (b *Bot) Say(channel, text string) {
	randomIndex := rand.Intn(len(b.worker))
	b.worker[randomIndex].client.Say(channel, text)
}

// Connect startup the logger and bot
func (b *Bot) Connect() {
	b.startTime = time.Now()
	client := b.newClient()
	b.initialJoins()

	if strings.HasPrefix(b.cfg.Username, "justinfan") {
		log.Info("[bot] joining as anonymous user")
	} else {
		log.Info("[bot] joining as user " + b.cfg.Username)
	}

	log.Fatal(client.Connect())
}

func (b *Bot) Part(channelNames ...string) {
	for _, channelName := range channelNames {
		log.Info("[bot] leaving " + channelName)

		for _, worker := range b.worker {
			worker.client.Depart(channelName)
		}
	}
}

func (b *Bot) Join(channelNames ...string) {
	for _, channel := range channelNames {

		joined := false
		for _, worker := range b.worker {
			if len(worker.joinedChannels) < 50 {
				log.Info("[bot] joining " + channel)
				worker.client.Join(channel)
				worker.joinedChannels = append(worker.joinedChannels, channel)
				joined = true
				break
			}
		}
		if !joined {
			client := b.newClient()
			go client.Connect()
			b.Join(channel)
		}
	}
}

func (b *Bot) newClient() *twitch.Client {
	client := twitch.NewClient(b.cfg.Username, "oauth:"+b.cfg.OAuth)
	if b.cfg.BotVerified {
		client.SetJoinRateLimiter(twitch.CreateVerifiedRateLimiter())
	}

	b.worker = append(b.worker, &worker{client, []string{}})
	log.Infof("[bot] creating new twitch connection, new total: %d", len(b.worker))

	client.OnPrivateMessage(b.handlePrivateMessage)
	client.OnUserNoticeMessage(b.handleUserNotice)
	client.OnClearChatMessage(b.handleClearChat)

	return client
}

func (b *Bot) initialJoins() {
	for _, channel := range b.channels {
		b.Join(channel.Login)
	}
}

func (b *Bot) handlePrivateMessage(message twitch.PrivateMessage) {
	b.handlePrivateMessageCommands(message)

	if b.cfg.IsOptedOut(message.User.ID) || b.cfg.IsOptedOut(message.RoomID) {
		return
	}

	go func() {
		err := b.logger.LogPrivateMessageForUser(message.User, message)
		if err != nil {
			log.Error(err.Error())
		}
	}()

	go func() {
		err := b.logger.LogPrivateMessageForChannel(message)
		if err != nil {
			log.Error(err.Error())
		}
	}()
}

func (b *Bot) handleUserNotice(message twitch.UserNoticeMessage) {
	if b.cfg.IsOptedOut(message.User.ID) || b.cfg.IsOptedOut(message.RoomID) {
		return
	}

	go func() {
		err := b.logger.LogUserNoticeMessageForUser(message.User.ID, message)
		if err != nil {
			log.Error(err.Error())
		}
	}()

	if _, ok := message.Tags["msg-param-recipient-id"]; ok {
		go func() {
			err := b.logger.LogUserNoticeMessageForUser(message.Tags["msg-param-recipient-id"], message)
			if err != nil {
				log.Error(err.Error())
			}
		}()
	}

	go func() {
		err := b.logger.LogUserNoticeMessageForChannel(message)
		if err != nil {
			log.Error(err.Error())
		}
	}()
}

func (b *Bot) handleClearChat(message twitch.ClearChatMessage) {
	if b.cfg.IsOptedOut(message.TargetUserID) || b.cfg.IsOptedOut(message.RoomID) {
		return
	}

	if message.BanDuration == 0 {
		count, ok := b.clearchats.Load(message.RoomID)
		if !ok {
			count = 0
		}
		newCount := count.(int) + 1
		b.clearchats.Store(message.RoomID, newCount)

		go func() {
			time.Sleep(time.Second * 1)
			count, ok := b.clearchats.Load(message.RoomID)
			if ok {
				b.clearchats.Store(message.RoomID, count.(int)-1)
			}
		}()

		if newCount > 50 {
			if newCount == 51 {
				log.Infof("Stopped recording CLEARCHAT permabans in: %s", message.Channel)
			}
			return
		}
	}

	go func() {
		err := b.logger.LogClearchatMessageForUser(message.TargetUserID, message)
		if err != nil {
			log.Error(err.Error())
		}
	}()

	go func() {
		err := b.logger.LogClearchatMessageForChannel(message)
		if err != nil {
			log.Error(err.Error())
		}
	}()
}
