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
| Sunday | 7:30 AM | league | Injury Monitor *(optional, `MONITOR_REPORT`, on by default)* | Rostered players who are OUT, flagged Doubtful/IR-eligible, or sitting in an IR slot without a qualifying designation |
| Sunday | 4:00 PM | ET | Scoreboard | In-progress scores for every matchup |
| Sunday | 4:00 PM | ET | Close Scores | Matchups still within the close-game threshold (`CLOSE_SCORES_THRESHOLD`, default 15 pts) |
| Sunday | 8:00 PM | ET | Scoreboard | In-progress scores, evening update |
| Sunday | 8:00 PM | ET | Close Scores | Matchups still within the close-game threshold, evening update |
| Monday | 7:30 AM | league | Weekday Scoreboard | Current scores plus each matchup's approximate projected final score |
| Monday | 6:30 PM | ET | Close Scores | Matchups still within the close-game threshold, before Monday Night Football wraps |
| Tuesday | 7:30 AM | league | Final | Final scores and trophies (blowout, closest game, luck, over/underachiever, best/worst manager) for the week that just finished |
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
game threshold, team abbreviations, AI weekly recap provider, web portal).

## Web Portal

Set `PORTAL_PASSWORD` to turn on a small password-protected web UI, served
on `PORTAL_PORT` (default `8080`), for changing four things without editing
`.env` or restarting the bot:

- Turning any individual scheduled message (see the table above) on or off
- Which day(s) of the week the Waiver Report posts on
- The timezone used for "league time" messages
- The AI Weekly Recap's system prompt

Anything changed in the portal takes priority over the matching `.env`
value; anything left alone keeps using its `.env`-derived default (or, for
settings with no `.env` equivalent, a built-in default). A "Reset
everything to .env defaults" button on the dashboard clears all overrides
at once.

Settings are persisted to `SETTINGS_FILE` (default `data/settings.json`) so
they survive a restart — **in Docker this must be on a mounted volume**
(see [docker-compose.yml](docker-compose.yml)'s `fantasy_bot_data` volume),
or every redeploy will silently reset the portal's overrides back to
`.env`.

The portal itself speaks plain HTTP, with no built-in TLS — if you expose
it beyond your home network, put it behind a reverse proxy (e.g. Caddy,
Nginx Proxy Manager, a Cloudflare Tunnel) that terminates HTTPS.

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
   causes a `.env not found` deploy error.
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
