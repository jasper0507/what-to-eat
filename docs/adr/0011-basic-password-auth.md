# 0011. Basic first-party username/password auth (no third-party IdP)

## Status

Accepted

## Context

Accounts are mandatory. Options included OAuth, email magic links, SMS OTP, or first-party credentials. The operator wants minimal trouble and cost, and **no third-party identity providers** for login.

## Decision

v1 auth is **first-party only**:

- **Register / login** with **username + password** (email optional as a field later; not required for v1).
- Sessions via **server-issued session cookie or JWT** stored after password verify.
- Passwords **hashed** at rest (e.g. argon2/bcrypt); never store plaintext.
- **No** GitHub/Google OAuth, **no** SMS, **no** external magic-link email provider for v1.

Account recovery in v1 can be out of band (operator reset) or deferred; not a blocker for personal/family use of a private deploy.

## Consequences

- Zero IdP fees and simpler compliance surface for auth.
- User must remember credentials; no "Login with Google".
- Need standard hardening: rate limit login, HTTPS, secure cookies, password minimum rules.
- NIM remains a third-party **for onboarding LLM only**, unrelated to identity.
