# open_crm

A clean CRM MVP built with a Go backend, a JavaScript React frontend, and Postgres.

## Local commands

```bash
cp .env.example .env
make db-up
make api-dev
make web-dev
make test
```

## GitHub Actions deployment

Frontend:
- `.github/workflows/frontend-pages.yml` builds `apps/web` and deploys `apps/web/dist` to GitHub Pages.
- In the repo settings, set Pages to use GitHub Actions as the source.

Backend + database:
- `.github/workflows/backend-deploy.yml` syncs the repo to `aeml@ssh.mendola.tech:~/open_crm`, writes `.env.production` from a GitHub secret, then runs `docker compose -f docker-compose.deploy.yml --env-file .env.production up -d --build` on the remote host.

Required GitHub secrets:
- `SSH_PRIVATE_KEY`
- `DEPLOY_ENV`

Example `DEPLOY_ENV` contents:
```env
POSTGRES_DB=open_crm
POSTGRES_USER=open_crm
POSTGRES_PASSWORD=change-me
API_PORT=8080
SESSION_COOKIE_SECRET=change-me
GO_ENV=production
```

Remote host requirements:
- Docker with Compose plugin installed
- user `aeml` able to run Docker
- repo deploy target directory `~/open_crm`
