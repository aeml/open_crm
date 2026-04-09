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
POSTGRES_PASSWORD=***
API_PORT=18089
SESSION_COOKIE_SECRET=***
ALLOWED_ORIGINS=https://crm.mendola.tech
GO_ENV=production
```

Remote host requirements:
- Docker with Compose plugin installed
- user `aeml` able to run Docker
- repo deploy target directory `~/open_crm`

## Production backend host

The Go API should be exposed publicly as:
- `https://crmserver.mendola.tech`

The container should bind only on loopback on the server:
- `127.0.0.1:18089 -> container:8080`

That is the right shape. Let nginx face the internet. Do not expose the Go container directly on `0.0.0.0`.

## Nginx config for crmserver.mendola.tech

Create:
- `/etc/nginx/sites-available/crmserver.mendola.tech`

Use this server block:
```nginx
server {
    listen 80;
    listen [::]:80;
    server_name crmserver.mendola.tech;

    location / {
        proxy_pass http://127.0.0.1:18089;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 60s;
    }
}
```

Enable it:
```bash
sudo ln -s /etc/nginx/sites-available/crmserver.mendola.tech /etc/nginx/sites-enabled/crmserver.mendola.tech
sudo nginx -t
sudo systemctl reload nginx
```

## SSL certificate with certbot

Once DNS for `crmserver.mendola.tech` points to the server and nginx is serving port 80:
```bash
sudo apt-get update
sudo apt-get install -y certbot python3-certbot-nginx
sudo certbot --nginx -d crmserver.mendola.tech
```

Then verify renewal timer exists:
```bash
systemctl status certbot.timer
```

Test renewal:
```bash
sudo certbot renew --dry-run
```

## UFW rules

Use UFW like this:
```bash
sudo ufw allow OpenSSH
sudo ufw allow 'Nginx Full'
sudo ufw deny 18089/tcp
sudo ufw enable
sudo ufw status verbose
```

Notes:
- port `18089` should not be publicly reachable
- nginx on `80/443` should be the only public entrypoint for the backend
- because Docker binds the app to `127.0.0.1:18089`, public exposure is already avoided at the compose layer too

## CORS policy

Set this in `DEPLOY_ENV`:
```env
ALLOWED_ORIGINS=https://crm.mendola.tech
```

That means the backend at `crmserver.mendola.tech` will only emit CORS allow headers for the frontend origin `https://crm.mendola.tech`.

If your frontend ends up living somewhere else later, update it to that exact origin, for example:
```env
ALLOWED_ORIGINS=https://aeml.github.io
```

Do not use `*` for CORS here. That would be sloppy and wrong for an authenticated app.
