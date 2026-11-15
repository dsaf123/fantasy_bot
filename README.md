# fantasy_bot

A Discord bot for a Sleeper fantasy football league. It runs on its own
all week long, posting a rotating cast of reports — live scores, close-game
callouts, standings, power rankings, schedule-luck ("fortune") rankings,
matchup previews, waiver activity, an injury monitor, and an optional
AI-written weekly recap — to a Discord channel via an incoming webhook, on
a fixed weekly schedule timed around actual NFL game windows (see
[Scheduled messages](#scheduled-messages) below). It can also run any single
report on demand from the command line (`-report=<name>`) or print reports
to stdout without posting (`-dry-run`), and it's data-source-only: it reads
league, roster, matchup, and transaction data from Sleeper's public API and
never touches your roster.

## Scheduled messages

Every row below is one message the bot posts, in chronological order
starting Wednesday morning — when waivers clear and the next matchups are
set, effectively the start of the fantasy week. Times are either **ET**
(fixed to `America/New_York`, matching actual NFL game windows regardless
of league timezone — mirrors gamedaybot's behavior) or **league** (the
`TIMEZONE` env var, default `America/New_York`). Rows marked *optional*
only fire if the noted setting is enabled.

| Day | Time | Zone | Message | What it posts |
| --- | --- | --- | --- | --- |
| Wednesday | 7:30 AM | league | Standings | Current win-loss-tie standings |
| Wednesday | 7:30 AM | league | Win Matrix | Standings if every team had played every other team every week |
| Wednesday | 7:31 AM | league | Waiver Report | Completed waiver/free-agent adds and drops with FAAB bids *(daily instead of Wednesday-only if `DAILY_WAIVER=true`)* |
| Thursday | 7:30 PM | ET | Matchups | Upcoming week's pairings plus each matchup's projected scoreboard |
| Friday | 7:30 AM | league | Weekday Scoreboard | Current scores plus each matchup's approximate projected final score |
| Sunday | 7:30 AM | league | Injury Monitor *(optional, `MONITOR_REPORT`, on by default)* | Starting lineup problems to fix before kickoff: empty slots, starters on bye or without an NFL team, and starters with an injury designation (Questionable, Doubtful, Out, IR, PUP, Suspended). Uses freshly fetched injury data; bench and IR slots, and games already under way, are ignored. Pings [linked](#team-links) managers of flagged teams |
| Sunday | 11:45 AM | ET | Injury Monitor update *(same toggle)* | Re-checks lineups after teams announce inactives; posts the current list again only if something new turned up since the morning, pinging just those managers |
| Sunday | 4:00 PM | ET | Scoreboard | In-progress scores for every matchup |
| Sunday | 4:00 PM | ET | Close Scores | Matchups still within the close-game threshold (`CLOSE_SCORES_THRESHOLD`, default 15 pts) |
| Sunday | 8:00 PM | ET | Scoreboard | In-progress scores, evening update |
| Sunday | 8:00 PM | ET | Close Scores | Matchups still within the close-game threshold, evening update |
| Monday | 7:30 AM | league | Weekday Scoreboard | Current scores plus each matchup's approximate projected final score |
| Monday | 6:30 PM | ET | Close Scores | Matchups still within the close-game threshold, before Monday Night Football wraps |
| Tuesday | 7:30 AM | league | Final | Final scores and trophies (blowout, closest game, luck, over/underachiever, best/worst manager) for the week that just finished |
| Tuesday | 7:31 AM | league | Bad Management | Bar chart of points scored vs. points left on the bench for the week that just finished, ranked worst manager first |
| Tuesday | 9:00 AM | league | Trophy Case | Season-long crosstab image of every team's trophy counts, tallied through the week Final just posted |
| Tuesday | 6:30 PM | league | Power Rankings | Season-long power ranking score and rank for each team |
| Tuesday | 6:30 PM | league | Power Rankings Chart | Season-trend line chart image (skipped until at least two weeks have been scored) |
| Tuesday | 6:31 PM | league | Fortune Index | Schedule-luck ranking — who's over/underperformed based on opponent strength |
| Tuesday | 6:45 PM | league | AI Weekly Recap *(optional, requires `LLM_API_KEY`)* | LLM-written newsletter covering streaks, rivalries, the playoff race, and the week's lineup regret |

In addition, `INIT_MSG` (if set) is posted once on startup to confirm the
bot is wired up correctly — it isn't part of the weekly schedule above.

Whether each message above posts at all, which day(s) the Waiver Report
uses, the timezone, and the AI Weekly Recap's prompt can all be overridden
at runtime from the [web portal](#web-portal) below, without editing `.env`
or restarting the bot.

## Configuration

Copy [.env.example](.env.example) to `.env` and fill in at least:

- `LEAGUE_ID` — your Sleeper league ID (the numeric ID in the league's URL)
- `DISCORD_WEBHOOK_URL` — an incoming webhook URL for the channel to post to

See `.env.example` for the full list of optional settings (timezone, close
game threshold, team abbreviations, AI weekly recap provider, web portal,
Discord slash commands).

## Web Portal

Set `PORTAL_PASSWORD` to turn on a small password-protected web UI for
changing these without editing `.env` or restarting the bot:

- Turning any individual scheduled message (see the table above) on or off
- Which day(s) of the week the Waiver Report posts on
- The timezone used for "league time" messages
- The AI Weekly Recap's system prompt
- Whether the bot answers [@mentions](#mention-replies) in Discord
- Removing [team links](#team-links) (adding them needs Discord's
  `/settings link`, which can look up server members)

**The portal is entirely off unless `PORTAL_PASSWORD` is set.** `PORTAL_PORT`
(default `8080`) only picks which port the portal listens on *if* it's
enabled — setting `PORTAL_PORT` without `PORTAL_PASSWORD` does nothing, and
is a common trap when you're changing the port to dodge a conflict (see
[Portainer](#portainer) below) and forget the portal also needs the password
var. On startup the bot logs exactly one of `portal: PORTAL_PASSWORD not
set, portal disabled` or `portal: starting on :<port>`, so check the
container logs for one of those two lines if the portal won't load.

Anything changed in the portal takes priority over the matching `.env`
value; anything left alone keeps using its `.env`-derived default (or, for
settings with no `.env` equivalent, a built-in default). A "Reset
everything to .env defaults" button on the dashboard clears all overrides
at once. [Team links](#team-links) aren't `.env` overrides, so resets keep
them.

Settings are persisted to `SETTINGS_FILE` (default `data/settings.json`) so
they survive a restart — **in Docker this must be on a mounted volume**
(see [docker-compose.yml](docker-compose.yml)'s `fantasy_bot_data` volume),
or every redeploy will silently reset the portal's overrides back to
`.env`.

The portal itself speaks plain HTTP, with no built-in TLS — if you expose
it beyond your home network, put it behind a reverse proxy (e.g. Caddy,
Nginx Proxy Manager, a Cloudflare Tunnel) that terminates HTTPS.

## Discord Commands

Set `DISCORD_BOT_TOKEN` to manage the same overrides as the [web
portal](#web-portal) — which scheduled messages are on, the Waiver Report's
day(s), the timezone, and the AI Weekly Recap's prompt — with a `/settings`
slash command instead, right from Discord. Anyone with the server's "Manage
Server" permission can run it; access isn't controlled by a separate
password like the portal's, but by that Discord permission (server admins
can further customize who's allowed from Discord's own Integrations
settings). The command only works in a server, not in DMs.

Unlike the portal, this doesn't need the bot to be reachable from the
internet — it connects out to Discord the same way the bot already reads
Sleeper's API, so it works just as well fully behind a home network.

| Command | What it does |
| --- | --- |
| `/settings view` | Show every current setting and whether it's an override or a `.env` default |
| `/settings job` | Turn one scheduled message on or off — `name` is a dropdown of every message in the [schedule table](#scheduled-messages) above, `enabled` a true/false toggle |
| `/settings waiver-days days:<list>` | Set which day(s) the Waiver Report posts — free text, e.g. `wed`, `sun,wed,fri`, `all`, or `none` |
| `/settings timezone value:<zone>` | Set the timezone used for "league time" messages — free text, e.g. `America/Chicago` |
| `/settings recap-prompt` | Set the AI Weekly Recap's system prompt via the free-text `prompt` option (omit it to just view the current one); capped at 1900 characters — use the web portal for anything longer |
| `/settings mentions enabled:<true/false>` | Turn [@mention replies](#mention-replies) on or off |
| `/settings link who:<member> team:<team>` | Record which league team a Discord member manages — `team` autocompletes from your Sleeper league. See [Team links](#team-links) |
| `/settings unlink who:<member>` | Remove a member's team link |
| `/settings reset` | Reset overrides back to `.env` defaults (team links are kept) — `scope` is a dropdown for either everything or just the recap prompt |

To turn this on:

1. Create a bot user at the [Discord Developer Portal](https://discord.com/developers/applications)
   (**New Application**, then **Bot** → **Reset Token** to get its token).
2. Under **OAuth2 → URL Generator**, check the **bot** and
   **applications.commands** scopes, then open the generated URL to invite
   it to your server. `/settings` itself needs no bot permissions (access is
   controlled by "Manage Server" instead), but for [mention
   replies](#mention-replies) also check **Send Messages**, **Read Message
   History**, and **Add Reactions** under Bot Permissions.
3. Set `DISCORD_BOT_TOKEN` to the token from step 1. `DISCORD_GUILD_ID`
   (your server's ID) is optional — without it, new/changed commands can
   take up to an hour to appear (Discord's global-command propagation
   delay); with it, they show up within seconds.
4. Restart the bot. It logs `discordbot: connected as ..., /settings and
   /roast registered ...` once it's up, or `discordbot: DISCORD_BOT_TOKEN not set,
   slash commands and @mention replies disabled` if the token is missing.
   The same bot also serves [mention replies](#mention-replies) and
   [`/roast`](#roast).

### Mention replies

With both `DISCORD_BOT_TOKEN` and `LLM_API_KEY` set, league members can
@mention the bot with a question and it replies in the channel, e.g.
`@fantasy_bot who has the toughest playoff schedule?` or `@fantasy_bot is
my team cooked?`. Answers are written by the same AI provider as the
weekly recap, based on a fresh snapshot of the league: standings, streaks,
head-to-head series, the playoff race, this week's scores, and every
team's roster with injury designations. With [team links](#team-links) set
up, it also knows whose team is whose. Replying to one of the bot's
answers (with the reply ping left on) asks a follow-up, with the message
you replied to included as context.

- It's on by default whenever `LLM_API_KEY` is set; turn it off from the
  web portal's "Chat replies" card or `/settings mentions enabled:false`.
- Each person can ask once every 20 seconds (shared with
  [`/roast`](#roast)); extra questions in that window get a ⏳ reaction
  instead of an answer. Every answer is one LLM call.
- It only answers in servers (not DMs), never pings anyone but the person
  who asked, and ignores other bots — including the report webhook.
- Replies come from the bot user rather than the report webhook. To make
  them look like the same bot, give the bot application (Developer Portal →
  **Bot**) the same name and avatar as your webhook.
- The bot needs **Send Messages** and **Read Message History** in the
  channel (plus **Add Reactions** for the ⏳). If you invited it before
  this feature existed, it may not have them: re-invite it with those
  permissions checked, or grant them to its role in the channel or server
  settings. It doesn't need the
  privileged Message Content intent: Discord always shares a message's text
  with a bot that's @mentioned in it.
- When typing `@`, pick the bot itself from the members list rather than
  its same-named role. Role mentions are recognized, but Discord may hide
  the message text from the bot, in which case it replies asking you to
  mention it again.

### Roast

`/roast` has the bot publicly roast a league member's fantasy team — their
record, losing streaks, bench blunders, injured players, playoff odds —
using the same league snapshot and AI provider as [mention
replies](#mention-replies). It needs `DISCORD_BOT_TOKEN` and `LLM_API_KEY`.

| Option | What it does |
| --- | --- |
| `who` | The Discord member to roast. They get pinged, and their [linked team](#team-links) is the one roasted. If they aren't linked, the bot tries to match their Discord names to a Sleeper manager or team. |
| `team` | The league team to roast, picked from an autocomplete list. Whoever is [linked](#team-links) to it gets pinged. |

Give at least one of the two. If the bot can't tell which team is the
target's (no link, and their Discord name doesn't match), it says so and
keeps the roast general rather than pinning someone else's stats on them. It sticks to fantasy football: nothing personal.

Anyone in the server can run it, and each person can roast once every 20
seconds (shared with mention replies). It's not a portal setting: to limit
who can use it, or turn it off, use **Server Settings → Integrations →
fantasy_bot → /roast** in Discord.

### Team links

Discord and Sleeper names often don't match, so the bot can't always tell
whose team is whose. An admin can link each league member once with
`/settings link who:@member team:<team>`. The team picker autocompletes
from your Sleeper league, and links survive team renames since they're
stored by Sleeper user rather than team name.

Once someone is linked:

- `/roast who:@them` roasts their team, and `/roast team:<their team>`
  pings them.
- @mention answers know the asker's team (so "is my team cooked?" works)
  and the team of anyone they @mention.
- The Sunday Injury Monitor pings them when their starting lineup has
  something to fix.

`/settings view` and the web portal list the links. Remove one with
`/settings unlink` or the portal's checkboxes. A team can be linked to more
than one member, e.g. co-managers.

## Running locally

```bash
go run . -dry-run
```

`-dry-run` prints reports to stdout instead of posting to Discord. Omit it,
and pass `-report=<name>` to run a single report and exit (see `-help` for
the full list), or run with no flags to start the always-on scheduler.

## Running with Docker

A prebuilt image is published to GHCR on every push to `main` via
[.github/workflows/docker-publish.yml](.github/workflows/docker-publish.yml):

```
ghcr.io/dsaf123/fantasy_bot:latest
```

### Docker Compose

```bash
docker compose up -d
```

[docker-compose.yml](docker-compose.yml) pulls the GHCR image; the
`environment:` block uses `${VAR}` substitution, which `docker compose`
fills in automatically from a `.env` file in the same directory. It also
publishes `PORTAL_PORT` (default `8080`) and mounts a `fantasy_bot_data`
named volume for the [web portal](#web-portal)'s settings — set
`PORTAL_PASSWORD` in `.env` to turn the portal on.

### Portainer

1. In Portainer, go to **Stacks → Add stack**.
2. Choose **Repository**, point it at this GitHub repo, and set the compose
   path to `docker-compose.yml` — or choose **Web editor** and paste the
   contents of `docker-compose.yml` directly.
3. In the **Environment variables** section of the stack form, add each
   variable from `.env.example` as a key/value pair (or use the "Load
   variables from .env file" option and paste the file's contents). Portainer
   uses these for the `${VAR}` substitution in the compose file — don't rely
   on `env_file: .env`, since Portainer doesn't write an actual `.env` file
   into the stack's directory just because you filled in that form, which
   causes a `.env not found` deploy error. Several settings, including the
   entire [web portal](#web-portal), are commented out in `.env.example`
   since they're opt-in — add `PORTAL_PASSWORD` explicitly if you want the
   portal (see the bolded note in that section: `PORTAL_PORT` alone does
   nothing).
4. Deploy the stack. Portainer will pull `ghcr.io/dsaf123/fantasy_bot:latest`
   and start the bot with `restart: unless-stopped`. If you set
   `PORTAL_PASSWORD`, the [web portal](#web-portal) is now reachable at
   `http://<host>:<PORTAL_PORT>` (default port `8080`), and its settings
   persist across redeploys in the stack's `fantasy_bot_data` volume.
5. To pick up new pushes to `main`, re-pull and redeploy the stack (Portainer
   has a "Pull and redeploy" button on the stack page), or enable Portainer's
   webhook for the stack and add it as a step in the GitHub Actions workflow.

**Note:** the first time the publish workflow runs, GitHub may create the
`fantasy_bot` package as private even though the repo is public. If Portainer
can't pull the image, go to the package's settings on GitHub
(`https://github.com/dsaf123?tab=packages`) and change its visibility to
public.

## Development

```bash
go build ./...
go test ./...
```
