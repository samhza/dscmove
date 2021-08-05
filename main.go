package main

import (
	"errors"
	"flag"
	"fmt"
	"sort"
	"strings"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/bot"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/utils/json/option"
)

type Bot struct {
	Ctx *bot.Context

	OwnerID discord.UserID
}

func (b *Bot) Addrole(m *gateway.MessageCreateEvent, from, to string) error {
	if m.Author.ID != b.OwnerID {
		return errors.New("you can't run this command")
	}
	if from == to {
		return errors.New("new/old role can't be the same")
	}
	if !m.GuildID.IsValid() {
		return errors.New("command must be ran in a server")
	}
	var rfrom, rto discord.RoleID
	roles, err := b.Ctx.Roles(m.GuildID)
	if err != nil {
		return fmt.Errorf("fetching roles: %v", roles)
	}
	for _, role := range roles {
		if role.Name == from {
			rfrom = role.ID
		} else if role.Name == to {
			rto = role.ID
		}
		if rto.IsValid() && rfrom.IsValid() {
			break
		}
	}
	if !rto.IsValid() {
		return fmt.Errorf("role '%s' not found", to)
	}
	if !rfrom.IsValid() {
		return fmt.Errorf("role '%s' not found", rfrom)
	}
	members, err := b.Ctx.Members(m.GuildID)
	if err != nil {
		return fmt.Errorf("fetching membors: %v", members)
	}
	errs := new(strings.Builder)
Outer:
	for _, mem := range members {
		if mem.User.ID == b.Ctx.Ready().User.ID {
			continue
		}
		hasfrom := false
		for _, r := range mem.RoleIDs {
			switch r {
			case rto:
				continue Outer
			case rfrom:
				hasfrom = true
			}
		}
		if !hasfrom {
			continue
		}
		roles := append(mem.RoleIDs, rto)
		if err := b.Ctx.ModifyMember(m.GuildID, mem.User.ID,
			api.ModifyMemberData{Roles: &roles}); err != nil {
			fmt.Fprintf(errs, "modifying member roles for %s: %v\n", mem.User.Username, err)
		}
	}
	if errs.Len() == 0 {
		return nil
	}
	return errors.New(errs.String())
}

func (b *Bot) Moveserver(m *gateway.MessageCreateEvent, from, to discord.GuildID) error {
	if m.Author.ID != b.OwnerID {
		return errors.New("you can't run this command")
	}
	ch, err := b.Ctx.Channel(m.ChannelID)
	if err != nil {
		return err
	}
	if ch.Type != discord.DirectMessage {
		return errors.New("command must be ran in DM")
	}
	roles, err := b.Ctx.Roles(from)
	if err != nil {
		return fmt.Errorf("fetching roles: %w", err)
	}
	roleids := make(map[discord.RoleID]discord.RoleID, len(roles))
	for _, r := range roles {
		if r.ID == discord.RoleID(from) {
			roleids[r.ID] = discord.RoleID(to)
			b.Ctx.ModifyRole(to, discord.RoleID(to), api.ModifyRoleData{
				Name:        option.NewNullableString(r.Name),
				Permissions: &r.Permissions,
				Color:       option.NewNullableColor(r.Color),
				Hoist:       &option.NullableBoolData{r.Hoist, true},
				Mentionable: &option.NullableBoolData{r.Mentionable, true},
			})
			continue
		}
		data := api.CreateRoleData{
			Name:        r.Name,
			Permissions: r.Permissions,
			Color:       r.Color,
			Hoist:       r.Hoist,
			Mentionable: r.Mentionable}

		newrole, err := b.Ctx.CreateRole(to, data)
		if err != nil {
			return fmt.Errorf("creating role: %w", err)
		}
		roleids[r.ID] = newrole.ID
		fmt.Println(r)
	}
	channels, err := b.Ctx.Channels(from)
	if err != nil {
		return fmt.Errorf("fetching channels: %w", err)
	}
	sort.Slice(channels, func(i, j int) bool {
		return channels[i].Position < channels[j].Position
	})
	chanids := make(map[discord.ChannelID]discord.ChannelID, len(channels))
	for _, c := range channels {
		data := api.CreateChannelData{
			Name:           c.Name,
			Type:           c.Type,
			Topic:          c.Topic,
			VoiceBitrate:   c.VoiceBitrate,
			VoiceUserLimit: c.VoiceUserLimit,
			UserRateLimit:  c.UserRateLimit,
			Position:       &c.Position,
			Permissions:    make([]discord.Overwrite, len(c.Permissions)),
		}
		data.CategoryID, _ = chanids[c.CategoryID]
		for i, o := range c.Permissions {
			if o.Type == discord.OverwriteRole {
				o.ID = discord.Snowflake(roleids[discord.RoleID(o.ID)])
			}
			data.Permissions[i] = o
		}
		newchan, err := b.Ctx.CreateChannel(to, data)
		if err != nil {
			return fmt.Errorf("creating channel: %v", err)
		}
		chanids[c.ID] = newchan.ID
	}
	return nil
}

func main() {
	token := flag.String("token", "", "discord bot token")
	prefix := flag.String("prefix", "!", "discord bot prefix")
	uid := flag.Int64("user", 0, "user id")
	flag.Parse()
	b := new(Bot)
	b.OwnerID = discord.UserID(*uid)
	bot.Run(*token, b, func(c *bot.Context) error {
		c.HasPrefix = bot.NewPrefix(*prefix)
		c.SilentUnknown = struct{ Command, Subcommand bool }{true, true}
		return nil
	})
}
