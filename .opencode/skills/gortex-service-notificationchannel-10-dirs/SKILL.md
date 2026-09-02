---
name: gortex-service-notificationchannel-10-dirs
description: "Work in the service/notificationchannel +10 dirs area — 578 symbols across 16 files (81% cohesion)"
---

# service/notificationchannel +10 dirs

578 symbols | 16 files | 81% cohesion

## When to Use

Use this skill when working on files in:
- ``
- `app/app.go`
- `internal/auth/provider.go`
- `internal/auth/providers/github/provider.go`
- `internal/auth/providers/google/provider.go`
- `internal/auth/providers/keycloak/provider.go`
- `internal/auth/registry.go`
- `internal/platform/entitlements/entitlements.go`
- `internal/platform/secretbox/secretbox.go`
- `internal/platform/secretbox/secretbox_test.go`
- `internal/service/notificationchannel/notificationchannel.go`
- `internal/service/notificationchannel/notificationchannel_test.go`
- `internal/service/notificationchannel/secretmask.go`
- `notifychannel/mattermost/mattermost.go`
- `notifychannel/notifychannel.go`
- `notifychannel/notifychannel_test.go`

## Key Files

| File | Symbols |
|------|---------|
| `` | io, ReadAll, ReadFull, aes, CutPrefix, ... |
| `app/app.go` | ok, st, logger, zone, ResolveStrategyNames, ... |
| `internal/auth/provider.go` | AvatarURL, Email, Provider, Identity, Subject, ... |
| `internal/auth/providers/github/provider.go` | displayName, client, email, err, body, ... |
| `internal/auth/providers/google/provider.go` | err, code, DisplayName, init, oauth2, ... |
| `internal/auth/providers/keycloak/provider.go` | DisplayName, issuer, err, Name, token, ... |
| `internal/auth/registry.go` | name, Register, factory, ProviderFactory |
| `internal/platform/entitlements/entitlements.go` | Entitlements |
| `internal/platform/secretbox/secretbox.go` | key, aead, aead, block, err, ... |
| `internal/platform/secretbox/secretbox_test.go` | ct, err, got, b2, TestSealOpenRoundTrip, ... |
| `internal/service/notificationchannel/notificationchannel.go` | Sender, err, hadPrev, ctx, EnabledNames, ... |
| `internal/service/notificationchannel/notificationchannel_test.go` | t, k, t, TestSaveRejectsEnablingWithoutSecret, err, ... |
| `internal/service/notificationchannel/secretmask.go` | secret, secretMaskingSender, Sender, secret, sender, ... |
| `notifychannel/mattermost/mattermost.go` | u, raw, err, raw, s, ... |
| `notifychannel/notifychannel.go` | New, Descriptor, Fields, Key, Descriptor, ... |
| `notifychannel/notifychannel_test.go` | d, t, TestDescriptorMayHaveNoSecret |

## Entry Points

- `app/app.go::New`

## Connected Communities

- **usecase/goal +36 dirs** (56 cross-edges)
- **auth +67 dirs** (16 cross-edges)
- **service/servicetest +33 dirs** (13 cross-edges)
- **auth +32 dirs** (7 cross-edges)
- **http/dto +36 dirs** (6 cross-edges)
- **service/activity +61 dirs** (5 cross-edges)
- **platform/eventbus +7 dirs** (4 cross-edges)
- **v1/goals +9 dirs** (3 cross-edges)
- **store/memberships +14 dirs** (3 cross-edges)
- **render/notify +5 dirs** (2 cross-edges)
- **. +3 dirs · GenerateInviteToken** (2 cross-edges)
- **auth +6 dirs** (2 cross-edges)
- **auth +1 dirs** (1 cross-edges)
- **. +1 dirs · IsValidHTTPURL** (1 cross-edges)
- **system/notificationchannels +16 dirs** (1 cross-edges)
- **store +14 dirs** (1 cross-edges)
- **activity/purge +12 dirs** (1 cross-edges)
- **auth · ResolveStrategyFactoryByName** (1 cross-edges)
- **. +3 dirs · As** (1 cross-edges)
- **auth · Resolve** (1 cross-edges)
- **http +1 dirs · Server** (1 cross-edges)

## How to Explore

```
analyze(operation:"communities", id:"community-265")
explore(operation:"context", task:"understand service/notificationchannel +10 dirs", format:"gcx")
relations(operation:"usages", target:{symbol:"app/app.go::New"}, format:"gcx")
```

_`format: "gcx"` returns the [GCX1 compact wire format](../../docs/wire-format.md) — round-trippable, ~27% fewer tokens than JSON. Drop it for JSON output; agents using `@gortex/wire` or the Go `github.com/gortexhq/gcx-go` package decode either._
