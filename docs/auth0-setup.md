# Auth0 Setup Guide for Conduit

This guide configures Auth0 as the identity provider for Conduit, enabling self-service signup, social login (Google, GitHub), and SSO.

## Prerequisites

- An Auth0 account ([signup free at auth0.com](https://auth0.com))
- Conduit running (locally or deployed)

## Step 1: Create Auth0 Application

1. Log in to [Auth0 Dashboard](https://manage.auth0.com)
2. Go to **Applications > Applications > Create Application**
3. Name: `Conduit`
4. Type: **Single Page Application**
5. Click **Create**

## Step 2: Configure Application Settings

In your new application's **Settings** tab:

- **Allowed Callback URLs**: `http://localhost:3000/auth/callback`
  - For production: `https://your-conduit-domain.com/auth/callback`
- **Allowed Logout URLs**: `http://localhost:3000`
- **Allowed Web Origins**: `http://localhost:3000`

Click **Save Changes**.

## Step 3: Create API

1. Go to **Applications > APIs > Create API**
2. Name: `Conduit API`
3. Identifier: `https://api.conduit.local` (this is the audience)
4. Signing Algorithm: RS256
5. Click **Create**

## Step 4: Enable Social Connections (Optional)

1. Go to **Authentication > Social**
2. Enable **Google** and/or **GitHub**
3. Follow the prompts to configure OAuth credentials

## Step 5: Configure Conduit

Set these environment variables:

```bash
CONDUIT_AUTH0_DOMAIN=your-tenant.us.auth0.com
CONDUIT_AUTH0_CLIENT_ID=your-client-id-from-step-1
CONDUIT_AUTH0_AUDIENCE=https://api.conduit.local
CONDUIT_AUTH0_REDIRECT_URI=http://localhost:3000/auth/callback
```

### Docker Compose

In `docker-compose.yml`, add to the backend service environment:

```yaml
environment:
  CONDUIT_AUTH0_DOMAIN: your-tenant.us.auth0.com
  CONDUIT_AUTH0_CLIENT_ID: your-client-id
  CONDUIT_AUTH0_AUDIENCE: https://api.conduit.local
  CONDUIT_AUTH0_REDIRECT_URI: http://localhost:3000/auth/callback
```

### Helm

In `values.yaml`:

```yaml
auth0:
  domain: your-tenant.us.auth0.com
  clientId: your-client-id
  audience: https://api.conduit.local
```

## Step 6: Test the Flow

1. Start Conduit: `make dev`
2. Open http://localhost:3000
3. You should see **Sign In** and **Sign Up** buttons
4. Click **Sign Up** — you'll be redirected to Auth0
5. Create an account (or use Google/GitHub)
6. After signup, you'll be redirected back to Conduit's onboarding wizard

## How It Works

1. Frontend redirects to Auth0 Universal Login
2. User signs up or logs in at Auth0
3. Auth0 redirects back to `/auth/callback` with an authorization code
4. Frontend sends code to `POST /api/v1/auth/callback`
5. Backend exchanges code with Auth0 for tokens
6. If new user: auto-provisions tenant, org, project, environments
7. Returns Conduit JWT with tenant context
8. Frontend stores token and redirects to onboarding (new) or dashboard (returning)

## Dev Mode (Without Auth0)

When `CONDUIT_AUTH0_DOMAIN` is not set, Conduit falls back to local email/password authentication. The login page shows email/password fields and a "Developer Mode" JWT paste option.

## Troubleshooting

### "Callback URL mismatch"
Ensure the callback URL in Auth0 matches exactly: `http://localhost:3000/auth/callback`

### "Unauthorized" after login
Check that `CONDUIT_AUTH0_AUDIENCE` matches the API identifier in Auth0.

### CORS errors
Add `http://localhost:3000` to **Allowed Web Origins** in Auth0 app settings.

### Token expired
Auth0 access tokens expire after the configured TTL. The frontend SDK handles refresh automatically.
