# fantasy_bot

Posts scheduled fantasy football reports (scores, standings, power rankings,
trophies, waiver activity, injury monitor, AI weekly recap) for a Sleeper
league to a Discord channel via webhook.

## Configuration

Copy [.env.example](.env.example) to `.env` and fill in at least:

- `LEAGUE_ID` — your Sleeper league ID (the numeric ID in the league's URL)
- `DISCORD_WEBHOOK_URL` — an incoming webhook URL for the channel to post to

See `.env.example` for the full list of optional settings (timezone, close
game threshold, team abbreviations, AI weekly recap provider).

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

[docker-compose.yml](docker-compose.yml) pulls the GHCR image and loads
config from `.env` in the same directory.

### Portainer

1. In Portainer, go to **Stacks → Add stack**.
2. Choose **Repository**, point it at this GitHub repo, and set the compose
   path to `docker-compose.yml` — or choose **Web editor** and paste the
   contents of `docker-compose.yml` directly.
3. Under the stack's environment variables, add the same variables from
   `.env.example` (or upload a `.env` file if you're using the web editor).
4. Deploy the stack. Portainer will pull `ghcr.io/dsaf123/fantasy_bot:latest`
   and start the bot with `restart: unless-stopped`.
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
