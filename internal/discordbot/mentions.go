package discordbot

import (
	"context"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"

	"fantasy_bot/internal/config"
	"fantasy_bot/internal/settings"
)

// LeagueAI is the AI-backed league chat this package exposes in Discord,
// implemented by bot.Bot.
type LeagueAI interface {
	// Answer replies to a free-form question about the league (see
	// bot.Bot.Answer). asker is the asker's display name; replyingTo is
	// the text of the message they replied to, if any.
	Answer(ctx context.Context, asker, question, replyingTo string) (string, error)
	// Roast writes a roast of a league member's team (see bot.Bot.Roast).
	Roast(ctx context.Context, requester string, targetNames []string, team string) (string, error)
	// TeamNames lists the league's teams for /roast's autocomplete.
	TeamNames(ctx context.Context) ([]string, error)
}

const (
	// aiCooldown is how long one user must wait between questions or
	// roasts (shared across both), since each is a paid LLM call plus a
	// burst of Sleeper API requests.
	aiCooldown = 20 * time.Second
	// answerTimeout bounds the whole answer, data fetch plus LLM call.
	answerTimeout = 2 * time.Minute
	// typingRefresh re-sends the "is typing…" indicator, which Discord
	// shows for ~10 seconds per call, while the answer is being written.
	typingRefresh = 8 * time.Second
	// messageLimit is Discord's per-message character cap.
	messageLimit = 2000
)

// unreadableReply covers a mention that arrives with no content at all.
// Discord only exempts messages that mention the bot *user* from the
// privileged Message Content intent; picking the bot's auto-created role
// from the @ autocomplete instead can arrive with its content blanked.
const unreadableReply = "I couldn't read that message. Try mentioning me again, picking me from the members list in the @ autocomplete (not my role), or reply to one of my messages."

const emptyQuestionReply = "Hey! Ask me anything about the league - standings, matchups, rosters, who's on a heater, who should be worried about the playoffs."

// mentionHandler answers questions when someone @mentions the bot in a
// server channel.
type mentionHandler struct {
	cfg  *config.Config
	mgr  *settings.Manager
	ai   LeagueAI
	cool *cooldowns

	// botRoles caches, per guild ID, the managed role Discord creates for
	// this bot when it joins a server (see botRoleIDs).
	botRolesMu sync.Mutex
	botRoles   map[string]map[string]bool
}

func newMentionHandler(cfg *config.Config, mgr *settings.Manager, ai LeagueAI, cool *cooldowns) *mentionHandler {
	return &mentionHandler{
		cfg:      cfg,
		mgr:      mgr,
		ai:       ai,
		cool:     cool,
		botRoles: make(map[string]map[string]bool),
	}
}

// handle is the discordgo MessageCreate handler. discordgo runs each event
// handler in its own goroutine, so blocking here for the LLM call is fine.
func (h *mentionHandler) handle(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore bots (including ourselves and the report webhook) and DMs:
	// answers are for the league's server, where the data is shared anyway.
	if m.Author == nil || m.Author.Bot || m.GuildID == "" || s.State.User == nil {
		return
	}
	botID := s.State.User.ID
	if !mentionsUser(m.Mentions, botID) && !h.mentionsBotRole(s, m.Message, botID) {
		return
	}
	if !h.mgr.Get().MentionRepliesEnabled(settings.DefaultMentionReplies(h.cfg)) {
		return
	}

	if strings.TrimSpace(m.Content) == "" && len(m.Attachments) == 0 {
		h.reply(s, m.Message, unreadableReply)
		return
	}
	question := cleanQuestion(m.Content, botID, m.Mentions)
	if question == "" {
		h.reply(s, m.Message, emptyQuestionReply)
		return
	}

	if !h.cool.allow(m.Author.ID, time.Now()) {
		// A reaction rather than a reply, so someone spamming questions
		// doesn't get the bot to spam the channel back.
		if err := s.MessageReactionAdd(m.ChannelID, m.ID, "⏳"); err != nil {
			log.Printf("discordbot: react to rate-limited mention: %v", err)
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), answerTimeout)
	defer cancel()
	go keepTyping(ctx, s, m.ChannelID)

	answer, err := h.ai.Answer(ctx, askerName(m.Message), question, referencedText(m.Message))
	cancel() // stop the typing indicator before replying
	if err != nil {
		log.Printf("discordbot: answer mention from %s: %v", m.Author.ID, err)
		answer = "Sorry, I couldn't come up with an answer right now - try again in a bit."
	}
	if strings.TrimSpace(answer) == "" {
		answer = "I've got nothing. 🤷"
	}
	h.reply(s, m.Message, answer)
}

// reply posts text as a reply to m, split across messages if needed. Only
// the asker is pinged: everything else (@everyone, roles, other users the
// model might name) is suppressed, since the text comes from an LLM.
func (h *mentionHandler) reply(s *discordgo.Session, m *discordgo.Message, text string) {
	for i, chunk := range splitMessage(text, messageLimit) {
		send := &discordgo.MessageSend{
			Content:         chunk,
			AllowedMentions: &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}},
		}
		if i == 0 {
			send.Reference = m.SoftReference()
			send.AllowedMentions.RepliedUser = true
		}
		if _, err := s.ChannelMessageSendComplex(m.ChannelID, send); err != nil {
			log.Printf("discordbot: reply to mention in channel %s: %v", m.ChannelID, err)
			return
		}
	}
}

// mentionsBotRole reports whether m mentions one of the bot's own managed
// roles. Typing "@BotName" in Discord often autocompletes to the role
// Discord auto-creates for the bot rather than the bot user itself, which
// arrives as a role mention, so both need to count as "mentioning the bot".
func (h *mentionHandler) mentionsBotRole(s *discordgo.Session, m *discordgo.Message, botID string) bool {
	if len(m.MentionRoles) == 0 {
		return false
	}
	botRoles := h.botRoleIDs(s, m.GuildID, botID)
	for _, id := range m.MentionRoles {
		if botRoles[id] {
			return true
		}
	}
	return false
}

// botRoleIDs returns the managed roles the bot holds in guildID, fetched
// once per guild and cached. discordgo v0.29 doesn't expose a role's
// bot_id tag, so this intersects the bot member's roles with the guild's
// managed roles instead - a bot's own integration role is the only managed
// role a bot normally holds.
func (h *mentionHandler) botRoleIDs(s *discordgo.Session, guildID, botID string) map[string]bool {
	h.botRolesMu.Lock()
	defer h.botRolesMu.Unlock()
	if roles, ok := h.botRoles[guildID]; ok {
		return roles
	}

	member, err := s.GuildMember(guildID, botID)
	if err != nil {
		log.Printf("discordbot: look up own roles in guild %s: %v", guildID, err)
		return nil // not cached, so the next mention retries
	}
	guildRoles, err := s.GuildRoles(guildID)
	if err != nil {
		log.Printf("discordbot: list roles in guild %s: %v", guildID, err)
		return nil
	}
	managed := make(map[string]bool, len(guildRoles))
	for _, r := range guildRoles {
		if r.Managed {
			managed[r.ID] = true
		}
	}
	roles := make(map[string]bool)
	for _, id := range member.Roles {
		if managed[id] {
			roles[id] = true
		}
	}
	h.botRoles[guildID] = roles
	return roles
}

// keepTyping shows "is typing…" in channelID until ctx is done.
func keepTyping(ctx context.Context, s *discordgo.Session, channelID string) {
	t := time.NewTicker(typingRefresh)
	defer t.Stop()
	for {
		if err := s.ChannelTyping(channelID); err != nil {
			log.Printf("discordbot: typing indicator: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func mentionsUser(users []*discordgo.User, id string) bool {
	for _, u := range users {
		if u != nil && u.ID == id {
			return true
		}
	}
	return false
}

// mentionTokenRE matches Discord's raw user (<@id>, <@!id>) and role
// (<@&id>) mention syntax.
var mentionTokenRE = regexp.MustCompile(`<@[!&]?(\d+)>`)

// cleanQuestion turns a message's raw content into the question text: the
// bot's own mention is removed, other user mentions become "@Name" so the
// model can tell who's being talked about, and role mentions (the bot's own
// integration role included - see mentionsBotRole) are dropped, since the
// model couldn't resolve them anyway.
func cleanQuestion(content, botID string, mentions []*discordgo.User) string {
	names := make(map[string]string, len(mentions))
	for _, u := range mentions {
		if u != nil {
			names[u.ID] = u.DisplayName()
		}
	}

	out := mentionTokenRE.ReplaceAllStringFunc(content, func(tok string) string {
		id := mentionTokenRE.FindStringSubmatch(tok)[1]
		isRole := strings.HasPrefix(tok, "<@&")
		switch {
		case isRole:
			return "" // the bot's own role, or some other role the model can't resolve
		case id == botID:
			return ""
		case names[id] != "":
			return "@" + names[id]
		default:
			return tok
		}
	})
	return strings.Join(strings.Fields(out), " ")
}

// askerName is the name the asker shows up as in the server: their server
// nickname if set, else their Discord display name.
func askerName(m *discordgo.Message) string {
	if m.Member != nil && m.Member.Nick != "" {
		return m.Member.Nick
	}
	return m.Author.DisplayName()
}

// referencedText returns "Author: content" for the message m replies to, or
// "" if it isn't a reply.
func referencedText(m *discordgo.Message) string {
	ref := m.ReferencedMessage
	if ref == nil || strings.TrimSpace(ref.Content) == "" {
		return ""
	}
	author := "Someone"
	if ref.Author != nil {
		author = ref.Author.DisplayName()
	}
	return author + ": " + ref.Content
}

// splitMessage breaks text into chunks of at most limit characters,
// preferring line boundaries, then word boundaries, then a hard cut.
func splitMessage(text string, limit int) []string {
	text = strings.TrimSpace(text)
	var chunks []string
	for len([]rune(text)) > limit {
		r := []rune(text)
		window := string(r[:limit])
		cut := strings.LastIndex(window, "\n")
		if cut <= 0 {
			cut = strings.LastIndex(window, " ")
		}
		if cut <= 0 {
			cut = len(window)
		}
		chunks = append(chunks, strings.TrimSpace(text[:cut]))
		text = strings.TrimSpace(text[cut:])
	}
	if text != "" {
		chunks = append(chunks, text)
	}
	return chunks
}

// cooldowns rate-limits each user to one question per window.
type cooldowns struct {
	mu     sync.Mutex
	window time.Duration
	last   map[string]time.Time
}

func newCooldowns(window time.Duration) *cooldowns {
	return &cooldowns{window: window, last: make(map[string]time.Time)}
}

// allow reports whether userID may ask now, recording the attempt if so.
func (c *cooldowns) allow(userID string, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t, ok := c.last[userID]; ok && now.Sub(t) < c.window {
		return false
	}
	c.last[userID] = now
	// Prune stale entries so the map can't grow without bound.
	for id, t := range c.last {
		if now.Sub(t) >= c.window {
			delete(c.last, id)
		}
	}
	return true
}
